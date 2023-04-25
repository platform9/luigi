package controllers

import (
	"context"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	"github.com/platform9/luigi/yoshi/pkg/cni"
	"github.com/platform9/luigi/yoshi/pkg/utils/constants"
	"github.com/platform9/luigi/yoshi/pkg/utils/iputils"
	"github.com/platform9/luigi/yoshi/pkg/utils/vmutils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// NetworkWizardReconciler reconciles a NetworkWizard object
type VMReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

type VMReqWrapper struct {
	Log         logr.Logger
	Client      client.Client
	needsUpdate bool
	vm          *kubevirtv1.VirtualMachine
	vmRef       string
	networks    []*plumberv1.NetworkWizard
}

func NewVMReqWrapper(log logr.Logger, client client.Client) *VMReqWrapper {
	reqInfo := new(VMReqWrapper)
	reqInfo.Log = log
	reqInfo.Client = client
	reqInfo.needsUpdate = false
	return reqInfo
}

func (req *VMReqWrapper) WithVM(vm *kubevirtv1.VirtualMachine) *VMReqWrapper {
	req.vm = vm
	req.vmRef = vmutils.GetVmRef(vm)
	return req
}

func (req *VMReqWrapper) WithNetworks(networks ...*plumberv1.NetworkWizard) *VMReqWrapper {
	if req.networks == nil {
		req.networks = []*plumberv1.NetworkWizard{}
	}
	req.networks = append(req.networks, networks...)
	return req
}

//+kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *VMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("virtualmachine", req.NamespacedName)
	log.Info("Reconciling VM")

	vm := &kubevirtv1.VirtualMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, vm); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("failed to get VM", "err", err)
			return ctrl.Result{}, nil
		}
		log.Info("Unknown error:", "err", err)
		return ctrl.Result{}, err
	}

	reqWrapper := NewVMReqWrapper(log, r.Client).WithVM(vm)

	if !vm.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(vm, constants.VMFinalizerName) {
			if err := r.ReconcileDeleteVM(ctx, reqWrapper); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(vm, constants.VMFinalizerName)
			if err := r.Client.Update(ctx, vm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if res, err := r.ReconcileVM(ctx, reqWrapper); !res.IsZero() || err != nil {
		return res, err
	}

	if reqWrapper.needsUpdate {
		if res, err := r.UpdateCRs(ctx, reqWrapper); !res.IsZero() || err != nil {
			return res, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *VMReconciler) ReconcileVM(ctx context.Context, req *VMReqWrapper) (ctrl.Result, error) {

	if err := r.ReconcilePublicIPBGP(ctx, req); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ensureServiceForVM(ctx, req); err != nil {
		return ctrl.Result{}, err
	}

	if res, err := r.ReconcileFixedIP(ctx, req); !res.IsZero() || err != nil {
		return res, err
	}

	if !vmutils.VMHasStaticMAC(req.vm) {
		MAC, _ := iputils.GenerateRandomMAC()
		vmutils.SetVMStaticMAC(req.vm, MAC)
	}

	// TODO: Add VM Profiles/flavors

	return ctrl.Result{}, nil
}

func (r *VMReconciler) UpdateCRs(ctx context.Context, req *VMReqWrapper) (ctrl.Result, error) {
	req.Log.Info("UpdateCRs")
	if res, err := r.UpdateVM(ctx, req); !res.IsZero() || err != nil {
		return res, err
	}

	if res, err := r.UpdateVMNetworks(ctx, req); !res.IsZero() || err != nil {
		return res, err
	}

	return ctrl.Result{}, nil
}

func (r *VMReconciler) UpdateVM(ctx context.Context, req *VMReqWrapper) (ctrl.Result, error) {
	req.Log.Info("UpdateVM")
	if !controllerutil.ContainsFinalizer(req.vm, constants.VMFinalizerName) {
		controllerutil.AddFinalizer(req.vm, constants.VMFinalizerName)
		req.Log.Info("Adding finalizer and updating")
	}

	running := true
	req.vm.Spec.Running = &running
	if err := r.Client.Update(ctx, req.vm); err != nil {
		return r.handleError(err)
	}
	return ctrl.Result{}, nil
}

func (r *VMReconciler) UpdateVMNetworks(ctx context.Context, req *VMReqWrapper) (ctrl.Result, error) {
	req.Log.Info("UpdateVMNetworks")
	for i := 0; i < len(req.networks) && req.networks[i] != nil; i++ {
		network := req.networks[i]
		if err := r.Client.Status().Update(ctx, network); err != nil {
			req.Log.Info("Error updating network", "err", err)
			return r.handleError(err)
		}
		req.Log.Info("new IP Allocations", "status", network.Status)
	}

	return ctrl.Result{}, nil
}

func (r *VMReconciler) ReconcileFixedIP(ctx context.Context, req *VMReqWrapper) (ctrl.Result, error) {
	req.Log.Info("ReconcileFixedIPs")
	if vmutils.VMHasFixedIP(req.vm) {
		req.Log.Info("VM Already has fixedIP reservation")
		return ctrl.Result{}, nil
	}

	networkName := vmutils.GetVMNetworkAnnotation(req.vm)
	if networkName == "" {
		req.Log.Info("VM has no network annotation, not reconciling")
		return ctrl.Result{}, nil
	}
	network := &plumberv1.NetworkWizard{}
	nsm := types.NamespacedName{Name: networkName, Namespace: req.vm.Namespace}
	if err := r.Client.Get(ctx, nsm, network); err != nil {
		if apierrors.IsNotFound(err) {
			req.Log.Info("failed to get network", "err", err)
			return ctrl.Result{}, nil
		}
		req.Log.Info("Unknown error:", "err", err)
		return ctrl.Result{}, err
	}

	req = req.WithNetworks(network)

	cidr := *network.Spec.CIDR
	allocations := network.Status.IPAllocations
	if allocations == nil {
		allocations = make(map[string]string)
	}

	newIp, err := iputils.AllocateIP(allocations, cidr)
	if err != nil {
		req.Log.Error(err, "cidr", cidr)
		return ctrl.Result{}, err
	}
	req.Log.Info("Reserving new IP", "IP", newIp)
	vmutils.SetVMFixedIP(req.vm, newIp)

	if network.Status.IPAllocations == nil {
		network.Status.IPAllocations = make(map[string]string)
	}
	network.Status.IPAllocations[newIp] = req.vm.Name
	req.needsUpdate = true

	return ctrl.Result{}, nil
}

func (r *VMReconciler) ReconcileDeleteVM(ctx context.Context, req *VMReqWrapper) error {
	req.Log.Info("Deleting VM...")

	if err := r.deletePublicIPForVM(ctx, req); err != nil {
		return err
	}

	if err := r.deleteServiceForVM(ctx, req); err != nil {
		return err
	}

	networks, err := r.GetNetworksForVM(ctx, req)
	if err != nil {
		req.Log.Error(err, "Failed to get networks for VM")
		return err
	}
	if networks == nil {
		// VM could be using default IPAM, no Fixed IP to cleanup
		req.Log.Info("VM has no network annotations to cleanup")
		return nil
	}

	req = req.WithNetworks(networks...)

	if err := r.DeleteIPAllocationsForVM(ctx, req); err != nil {
		req.Log.Error(err, "Failed to delete IPs for VM")
		return err
	}

	return nil
}

func (r *VMReconciler) GetNetworksForVM(ctx context.Context, req *VMReqWrapper) ([]*plumberv1.NetworkWizard, error) {
	networkName := vmutils.GetVMNetworkAnnotation(req.vm)
	if networkName == "" {
		req.Log.Info("VM has no network annotation, no IPAM to cleanup")
		return nil, nil
	}

	network := &plumberv1.NetworkWizard{}
	nsm := types.NamespacedName{Name: networkName, Namespace: req.vm.Namespace}
	if err := r.Client.Get(ctx, nsm, network); err != nil {
		if apierrors.IsNotFound(err) {
			req.Log.Info("failed to get network", "err", err)
			return nil, nil
		}
		req.Log.Info("Unknown error:", "err", err)
		return nil, err
	}

	return []*plumberv1.NetworkWizard{network}, nil
}

func (r *VMReconciler) DeleteIPAllocationsForVM(ctx context.Context, req *VMReqWrapper) error {
	for i := 0; i < len(req.networks) && req.networks[i] != nil; i++ {
		updateNetwork := false
		network := req.networks[i]
		for ip, vmName := range network.Status.IPAllocations {
			if vmName == req.vm.Name {
				req.Log.Info("Removing IP Allocation", "network", network.Name, "IP", ip)
				delete(network.Status.IPAllocations, ip)
				updateNetwork = true
			}
		}
		if updateNetwork {
			if err := r.Client.Status().Update(ctx, network); err != nil {
				req.Log.Info("Error updating network", "err", err)
				_, err := r.handleError(err)
				return err
			}
			updateNetwork = false
		}
	}
	return nil
}

func (r *VMReconciler) ReconcilePublicIPBGP(ctx context.Context, req *VMReqWrapper) error {
	opts := cni.CNIOpts{Client: r.Client, Log: req.Log}
	publicProvider := cni.NewPublicProvider(ctx, &opts)

	publicIP := vmutils.GetVMPublicIP(req.vm)
	if publicIP == "" {
		req.Log.Info("VM has no Public IP")

		// Check Service for VM to determine if it previously had an IP
		// We could store allocations in Public NetworkWizard, but since it's already in the Service
		// avoid storing redundant data in two different places
		nsm := types.NamespacedName{Name: req.vm.Name, Namespace: req.vm.Namespace}
		service := &corev1.Service{}
		if err := r.Client.Get(ctx, nsm, service); err != nil {
			req.Log.Error(err, "No Service for VM, likely new VM creation")
			return nil
		}

		// VM should only have 1 public IP
		if service.Spec.ExternalIPs != nil {
			oldPublicIP := service.Spec.ExternalIPs[0]
			if oldPublicIP != "" {
				if err := publicProvider.DelBGPPublicIP(ctx, oldPublicIP); err != nil {
					req.Log.Error(err, "failed to remove public IP from BGPConfig", "publicIP", publicIP)
					return err
				}
				req.Log.Info("Removed old Public IP", "IP", oldPublicIP)
			}

			service.Spec.ExternalIPs = nil
			if err := r.Client.Update(ctx, service); err != nil {
				req.Log.Error(err, "failed to remove externalIP from service")
				return err
			}
		}

		return nil
	}

	err := publicProvider.AddBGPPublicIP(ctx, publicIP)
	if err != nil {
		req.Log.Error(err, "failed to update publicIP", publicIP)
		return err
	}

	req.Log.Info("Added public IP", "publicIP", publicIP)

	return nil
}

func (r *VMReconciler) ensureServiceForVM(ctx context.Context, req *VMReqWrapper) error {
	publicIP := vmutils.GetVMPublicIP(req.vm)
	objMeta := metav1.ObjectMeta{Name: req.vm.Name, Namespace: req.vm.Namespace}
	service := &corev1.Service{ObjectMeta: objMeta}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		if service.Spec.Selector == nil {
			service.Spec.Selector = make(map[string]string)
		}

		service.Spec.Selector[constants.Pf9VMIServiceLabel] = req.vmRef

		// Set default ports if none present
		if service.Spec.Ports == nil {
			ports := []corev1.ServicePort{
				{
					Name:     "ssh",
					Port:     22,
					Protocol: "TCP",
				},
				{
					Name:     "http",
					Port:     80,
					Protocol: "TCP",
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: "TCP",
				},
			}
			service.Spec.Ports = ports
		}

		if publicIP != "" {
			service.Spec.ExternalIPs = []string{publicIP}
		}

		return nil
	})
	if err != nil {
		req.Log.Error(err, "Failed to create Service for VM", "vm", req.vmRef)
		return err
	}
	return nil
}

func (r *VMReconciler) deleteServiceForVM(ctx context.Context, req *VMReqWrapper) error {
	service := &corev1.Service{}
	service.ObjectMeta = metav1.ObjectMeta{Name: req.vm.GetName(), Namespace: req.vm.GetNamespace()}
	if err := r.Client.Delete(ctx, service); err != nil {
		if apierrors.IsNotFound(err) {
			req.Log.Info("Service not found for VM, nothing to delete", "service", service.GetName())
			return nil
		}
		req.Log.Error(err, "Failed to delete Service for VM", "service", service.GetName())
		return err
	}

	return nil
}

func (r *VMReconciler) deletePublicIPForVM(ctx context.Context, req *VMReqWrapper) error {
	publicIP := vmutils.GetVMPublicIP(req.vm)
	if publicIP == "" {
		req.Log.Info("VM has no Public IP")
		return nil
	}

	req.Log.Info("Deleting public IP from BGPConfig for VM", "IP", publicIP)
	opts := cni.CNIOpts{Client: r.Client, Log: req.Log}
	publicProvider := cni.NewPublicProvider(ctx, &opts)
	err := publicProvider.DelBGPPublicIP(ctx, publicIP)
	if err != nil {
		req.Log.Error(err, "failed to remove publicIP", publicIP)
		return err
	}

	return nil
}

func (r *VMReconciler) handleError(err error) (ctrl.Result, error) {
	if apierrors.IsConflict(err) {
		r.Log.Info("Conflict updating resource:", "err", err)
		return ctrl.Result{Requeue: true}, nil
	}
	if apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true}, nil
	}
	r.Log.Error(err, "unable to update resource")
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *VMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubevirtv1.VirtualMachine{}).
		Complete(r)
}

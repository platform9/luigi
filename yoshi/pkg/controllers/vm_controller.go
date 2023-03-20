package controllers

import (
	"context"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	"github.com/platform9/luigi/yoshi/pkg/utils/constants"
	"github.com/platform9/luigi/yoshi/pkg/utils/iputils"
	"github.com/platform9/luigi/yoshi/pkg/utils/vmutils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	log.Info("Inside VM controller!!")

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
	if !controllerutil.ContainsFinalizer(req.vm, constants.VMFinalizerName) {
		controllerutil.AddFinalizer(req.vm, constants.VMFinalizerName)
		req.Log.Info("Adding finalizer and updating")
		if err := r.Client.Update(ctx, req.vm); err != nil {
			r.Log.Error(err, "unable to update VM with finalizer")
			return ctrl.Result{Requeue: true}, err
		}
	}

	if res, err := r.ReconcileFixedIP(ctx, req); !res.IsZero() || err != nil {
		return res, err
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

	cidr := network.Spec.Cidr
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

func updateCR[T *plumberv1.NetworkWizard | *kubevirtv1.VirtualMachine](ctx context.Context, r client.Client, o T) (ctrl.Result, error) {
	if err := r.Update(ctx, any(o).(client.Object)); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		if apierrors.IsNotFound(err) {

			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubevirtv1.VirtualMachine{}).
		Complete(r)
}

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/platform9/luigi/yoshi/pkg/cni"
	"github.com/platform9/luigi/yoshi/pkg/utils/constants"
	"github.com/platform9/luigi/yoshi/pkg/utils/vmutils"
)

// NetworkWizardReconciler reconciles a NetworkWizard object
type NetworkWizardReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

type NetReqWrapper struct {
	Log         logr.Logger
	Client      client.Client
	needsUpdate bool
	network     *plumberv1.NetworkWizard
	cni         cni.CNIProvider
}

func NewNetReqWrapper(log logr.Logger, client client.Client) *NetReqWrapper {
	reqInfo := new(NetReqWrapper)
	reqInfo.Log = log
	reqInfo.Client = client
	reqInfo.needsUpdate = false
	return reqInfo
}

func (req *NetReqWrapper) WithNetwork(network *plumberv1.NetworkWizard) *NetReqWrapper {
	if req.network == nil {
		req.network = &plumberv1.NetworkWizard{}
	}
	req.network = network
	return req
}

func (req *NetReqWrapper) WithCNIProvider(provider cni.CNIProvider) *NetReqWrapper {
	req.cni = provider
	return req
}

//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkwizards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkwizards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkwizards/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *NetworkWizardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("network", req.NamespacedName)
	log.Info("Reconciling network")
	network := &plumberv1.NetworkWizard{}
	if err := r.Client.Get(ctx, req.NamespacedName, network); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	reqWrapper := NewNetReqWrapper(log, r.Client).WithNetwork(network)

	cni, err := cni.NewCNIProvider(ctx, network.Spec.Plugin, &cni.CNIOpts{Client: r.Client, Log: log})
	if err != nil {
		log.Error(err, "Faled to get CNI Provider")
		return ctrl.Result{}, err
	}

	reqWrapper = reqWrapper.WithCNIProvider(cni)

	if !network.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(network, constants.NetworkFinalizerName) {
			if err := r.ReconcileDelete(ctx, reqWrapper); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(network, constants.NetworkFinalizerName)
			if err := r.Client.Update(ctx, network); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	return r.ReconcileNetwork(ctx, reqWrapper)
}

func (r *NetworkWizardReconciler) ReconcileNetwork(ctx context.Context, req *NetReqWrapper) (ctrl.Result, error) {
	if req.network.Status.Created == true {
		req.Log.Info("Network.Status is created", "status", req.network.Status)
		if err := req.cni.VerifyNetwork(ctx, req.network.Name); err == nil {
			req.Log.Info("Network is already created")
			return ctrl.Result{}, nil
		}
	}

	r.Log.Info("Reconciling network", "network", req.network.Name, "spec", req.network.Spec)

	if !controllerutil.ContainsFinalizer(req.network, constants.NetworkFinalizerName) {
		controllerutil.AddFinalizer(req.network, constants.NetworkFinalizerName)
		if err := r.Client.Update(ctx, req.network); err != nil {
			r.Log.Error(err, "unable to update NetworkWizard with finalizer")
			return ctrl.Result{Requeue: true}, err
		}
	}

	if err := req.cni.CreateNetwork(ctx, req.network); err != nil {
		r.Log.Error(err, "Error creating network", "network", req.network.Spec)
		req.network.Status.Created = false
		req.network.Status.Reason = err.Error()
	} else {
		req.network.Status.Created = true
		r.Log.Info("Success creating network")
	}

	if err := r.Status().Update(ctx, req.network); err != nil {
		r.Log.Error(err, "unable to update NetworkWizard status")
		return ctrl.Result{}, err
	}

	r.Log.Info("Updated network status", "Status", req.network.Status)

	return ctrl.Result{}, nil
}

func (r *NetworkWizardReconciler) ReconcileDelete(ctx context.Context, req *NetReqWrapper) error {
	r.Log.Info("Deleting network", "network", req.network.Name, "spec", req.network.Spec)
	vmsOnNetwork, err := r.GetVMsForNetwork(req.network)
	if err != nil {
		return err
	}

	if len(vmsOnNetwork) > 0 {
		r.Log.Info("VMs exist on network", "vms", len(vmsOnNetwork))
		err := fmt.Errorf("VMs exist on network, can't delete")
		return err
	}

	r.Log.Info("Deleting network %s", "network", req.network.Name)
	if err := req.cni.DeleteNetwork(ctx, req.network.Name); err != nil {
		r.Log.Error(err, "Plugin failed to delete network resource")
		return err
	}
	return nil
}

func (r *NetworkWizardReconciler) GetVMsForNetwork(network *plumberv1.NetworkWizard) ([]*kubevirtv1.VirtualMachine, error) {
	var matchingVMs []*kubevirtv1.VirtualMachine
	vmList := &kubevirtv1.VirtualMachineList{}
	err := r.Client.List(context.Background(), vmList)
	if err != nil {
		r.Log.Error(err, "Failed to list VMs")
		return nil, err
	}
	for _, vm := range vmList.Items {
		if vmutils.GetVMNetworkAnnotation(&vm) == network.Name {
			matchingVMs = append(matchingVMs, &vm)
		}
	}
	return matchingVMs, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkWizardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.NetworkWizard{}).
		Complete(r)
}

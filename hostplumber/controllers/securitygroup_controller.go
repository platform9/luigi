/*
Copyright 2022.

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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	plumberv1 "hostplumber/api/v1"
)

// SecurityGroupReconciler reconciles a SecurityGroup object
type SecurityGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=securitygroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=securitygroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=securitygroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SecurityGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *SecurityGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log = r.Log.WithValues("securitygroup", req.NamespacedName)

	var secGroupReq = plumberv1.SecurityGroup{}
	if err := r.Get(ctx, req.NamespacedName, &secGroupReq); err != nil {
		log.Error(err, "unable to fetch SecurityGroup")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// name of our custom finalizer
	sgFinalizerName := "sgFinalizer"

	sgConfigList := secGroupReq.Spec.Rules //TODO: Change Rules according to whatever you decide in the spec

	// examine DeletionTimestamp to determine if object is under deletion
	if secGroupReq.ObjectMeta.DeletionTimestamp.IsZero() {

		if len(sgConfigList) > 0 {
			log.Info(" Reconcile triggered for create/update securityGroup")
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object.
			if !containsString(secGroupReq.GetFinalizers(), sgFinalizerName) {
				controllerutil.AddFinalizer(&secGroupReq, sgFinalizerName)
				log.Info("Adding Finalizer for sgcleanup")

				if err := r.Update(ctx, &secGroupReq); err != nil {
					log.Error(err, "Error Adding Finalizer")
					return ctrl.Result{}, err
				}
			}
		}
	} else {
		// The object is being deleted
		if containsString(secGroupReq.GetFinalizers(), sgFinalizerName) {
			if err := deleteFlows(sgConfigList); err != nil { //TODO make deleteFlows() function
				// return so that it can be retried
				return ctrl.Result{}, err
			}

			// remove finalizer from the list and update it.
			controllerutil.RemoveFinalizer(&secGroupReq, sgFinalizerName)
			log.Info(" Removing sgcleanup Finalizer in delete ")
			if err := r.Update(ctx, &secGroupReq); err != nil {
				log.Error(err, "removing Finalizer failed")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func deleteFlows(sgConfigList []plumberv1.Rule) { // TODO: Finish this function later

}

// SetupWithManager sets up the controller with the Manager.
func (r *SecurityGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.SecurityGroup{}).
		Complete(r)
}

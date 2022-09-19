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
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dhcpv1alpha1 "dhcp-controller/api/v1alpha1"
)

// DHCPServerReconciler reconciles a DHCPServer object
type DHCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=dhcp.plumber.k8s.pf9.io,resources=dhcpservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dhcp.plumber.k8s.pf9.io,resources=dhcpservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dhcp.plumber.k8s.pf9.io,resources=dhcpservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;delete;deletecollection;get;list;patch;update;watch
//+kubebuilder:rbac:groups=*,resources=virtualmachineinstances,verbs=get;list;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DHCPServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *DHCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ = log.FromContext(ctx)

	//log := r.Log.WithValues("dhcpservers", req.NamespacedName)

	var dhcpConfigReq = dhcpv1alpha1.DHCPServerList{}
	if err := r.List(ctx, &dhcpConfigReq); err != nil {
		log.Error(err, "unable to fetch DHCPServer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling")

	serverList := dhcpConfigReq.Items
	if len(serverList) > 0 {
		for _, server := range serverList {
			log.Infof("server %s", server.Name)
			result, err := r.ensureServer(req, server, r.backendDeployment(server))
			if result != nil {
				log.Error(err, "DHCP server Not ready")
				return *result, err
			}
		}
	} else {
		log.Info("serverList is Empty")
	}

	return ctrl.Result{}, nil
}

func (r *DHCPServerReconciler) ensureServer(request reconcile.Request,
	server dhcpv1alpha1.DHCPServer,
	dep *appsv1.Deployment,
) (*reconcile.Result, error) {

	// See if deployment already exists and create if it doesn't
	found := &appsv1.Deployment{}
	err := r.Get(context.TODO(), types.NamespacedName{
		Name:      dep.Name,
		Namespace: server.Namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create the deployment
		err = r.Create(context.TODO(), dep)

		if err != nil {
			// Deployment failed
			return &reconcile.Result{}, err
		} else {
			// Deployment was successful
			return nil, nil
		}
	} else if err != nil {
		// Error that isn't due to the deployment not existing
		return &reconcile.Result{}, err
	}

	return nil, nil
}

// backendDeployment is a code for Creating Deployment
func (r *DHCPServerReconciler) backendDeployment(v dhcpv1alpha1.DHCPServer) *appsv1.Deployment {

	size := int32(1)
	memReq := resource.NewQuantity(64*1024*1024, resource.BinarySI)
	memLimit := resource.NewQuantity(128*1024*1024, resource.BinarySI)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &size,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "dnsmasq", "name": v.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "dnsmasq", "name": v.Name},
					Annotations: map[string]string{"k8s.v1.cni.cncf.io/networks": v.Spec.NetworkName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "docker.io/ataa/dnsmasq:latest",
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{
									"NET_ADMIN",
								}}},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"memory": *memLimit,
							},
							Requests: corev1.ResourceList{
								"memory": *memReq,
							},
						},
						ImagePullPolicy: corev1.PullAlways,
						Name:            v.Name,
						Env: []corev1.EnvVar{
							{
								Name:  "NAD_NAME",
								Value: v.Spec.NetworkName,
							},
							{
								Name:  "BIND_INTERFACE_IP",
								Value: v.Spec.InterfaceIp,
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "dnsmasq-cfg",
							MountPath: "/etc/dnsmasq.d/",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "dnsmasq-cfg",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: v.Spec.ConfigMapName,
								},
								Items: []corev1.KeyToPath{{
									Key:  "dnsmasq.conf",
									Path: "dnsmasq.conf",
								}},
							},
						},
					}},
				},
			},
		},
	}

	controllerutil.SetControllerReference(&v, dep, r.Scheme)
	return dep
}

// SetupWithManager sets up the controller with the Manager.
func (r *DHCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpv1alpha1.DHCPServer{}).
		Complete(r)
}

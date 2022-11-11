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
	"fmt"
	cidr "github.com/apparentlymart/go-cidr/cidr"
	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"reflect"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
	"time"

	dhcpv1alpha1 "dhcp-controller/api/v1alpha1"
)

const (

	envVarDockerRegistry = "DOCKER_REGISTRY"
        defaultDockerRegistry = ""
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
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=create;delete;deletecollection;get;list;patch;update;watch

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

func (r *DHCPServerReconciler) genConfigMap(dhcpserver dhcpv1alpha1.DHCPServer) (error, *corev1.ConfigMap) {
	configMapData := make(map[string]string, 0)
	dnsmasqConfData := "port=0\n"

	for _, network := range dhcpserver.Spec.Networks {

		_, ipvNet, err := net.ParseCIDR(network.NetworkCIDR.CIDRIP)
		if err != nil {
			return err, nil
		}
		firstIP, lastIP := cidr.AddressRange(ipvNet)
		if network.NetworkCIDR.RangeStartIp == "" {
			network.NetworkCIDR.RangeStartIp = cidr.Inc(firstIP).String()
		}
		if network.NetworkCIDR.RangeEndIp == "" {
			network.NetworkCIDR.RangeEndIp = cidr.Dec(lastIP).String()
		}
		RangeNetMask := net.IP(ipvNet.Mask).String()

		if network.LeaseDuration == "" {
			network.LeaseDuration = "1h"
		}

		if network.VlanID == "" {
			dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-range=%s,%s,%s,%s\n", network.NetworkCIDR.RangeStartIp, network.NetworkCIDR.RangeEndIp, RangeNetMask, network.LeaseDuration)
		} else {
			dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-range=%s,%s,%s,%s,%s\n", network.VlanID, network.NetworkCIDR.RangeStartIp, network.NetworkCIDR.RangeEndIp, RangeNetMask, network.LeaseDuration)
		}
		if network.NetworkCIDR.GwAddress != "" {
			dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-option=3,%s\n", network.NetworkCIDR.GwAddress)
		}
	}
	configMapData["dnsmasq.conf"] = dnsmasqConfData
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpserver.Name,
			Namespace: dhcpserver.Namespace,
		},
		Data: configMapData,
	}
	return nil, configMap
}

func (r *DHCPServerReconciler) ensureServer(request reconcile.Request,
	server dhcpv1alpha1.DHCPServer,
	dep *appsv1.Deployment,
) (*reconcile.Result, error) {

	found := &appsv1.Deployment{}
	err, cm := r.genConfigMap(server)
	if err != nil {
		log.Error(err, "Failed to generate ConfigMap")
		return &reconcile.Result{}, err
	}

	oldcm := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: server.Namespace}, oldcm)
	if err != nil && errors.IsNotFound(err) {
		log.Info(" Creating Configmap...")
		controllerutil.SetControllerReference(&server, cm, r.Scheme)
		if err := r.Create(context.TODO(), cm); err != nil {
			log.Error(err, "Failed to create new ConfigMap")
			return &reconcile.Result{}, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get pre-existing ConfigMap")
	} else if err == nil {
		log.Info("Configmap exists with same name")
		if reflect.DeepEqual(cm.Data, oldcm.Data) == false {
			controllerutil.SetControllerReference(&server, cm, r.Scheme)
			if err := r.Update(context.TODO(), cm); err != nil {
				log.Error(err, "Failed to update ConfigMap")
				return &reconcile.Result{}, err
			}
			log.Info("server spec is changed, updating ConfigMap")
			// Delete the deployment, will be recreated with new configmap
			err = r.Delete(context.TODO(), dep)
			if err != nil {
				log.Error(err, "Failed to delete the deployment or there is none")
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: server.Namespace}, found)
			for {
				if err != nil && errors.IsNotFound(err) {
					break
				}
				err = r.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: server.Namespace}, found)
				time.Sleep(2 * time.Second)
			}
		} else {
			log.Info("server spec is unchanged, not updating ConfigMap")
		}
	}
	// See if deployment already exists and create if it doesn't
	err = r.Get(context.TODO(), types.NamespacedName{
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
	networkNames, interfaceIps := parseNetwork(v.Spec.Networks)
	size := int32(1)
	memReq := resource.NewQuantity(64*1024*1024, resource.BinarySI)
	memLimit := resource.NewQuantity(128*1024*1024, resource.BinarySI)
        dockerRegistry := getRegistry(envVarDockerRegistry, defaultDockerRegistry)
        image := dockerRegistry + "platform9/pf9-dnsmasq:v0.1" 
	log.Infof("image with registry %s", image)
       
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
					Annotations: map[string]string{"k8s.v1.cni.cncf.io/networks": networkNames},
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
						ImagePullPolicy: corev1.PullIfNotPresent,
						Name:            v.Name,
						Env: []corev1.EnvVar{
							{
								Name:  "BIND_INTERFACE_IP",
								Value: interfaceIps,
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "dnsmasq-cfg",
							MountPath: "/etc/dnsmasq.d/",
						}},
					}},
					ServiceAccountName: "dhcpserver-controller-manager",
					Volumes: []corev1.Volume{{
						Name: "dnsmasq-cfg",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: v.Name,
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

// getRegistry gets the override registry value or the default one
func getRegistry(envVar, defaultValue string) string {
	registry := os.Getenv(envVar)
	if registry == "" {
		registry = defaultValue
	}
	return registry
}

func parseNetwork(networks []dhcpv1alpha1.Network) (string, string) {
	var name []string
	var ip []string
	for _, network := range networks {
		name = append(name, network.NetworkName)
		ip = append(ip, network.InterfaceIp)
	}
	return strings.Join(name, ","), strings.Join(ip, ",")
}





// SetupWithManager sets up the controller with the Manager.
func (r *DHCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpv1alpha1.DHCPServer{}).
		Complete(r)
}

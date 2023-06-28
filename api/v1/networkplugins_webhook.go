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

package v1

import (
	"context"
	"fmt"
	"log"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var networkpluginslog = logf.Log.WithName("networkplugins-resource")

func (r *NetworkPlugins) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-plumber-k8s-pf9-io-v1-networkplugins,mutating=false,failurePolicy=fail,sideEffects=None,groups=plumber.k8s.pf9.io,resources=networkplugins,verbs=create;update,versions=v1,name=vnetworkplugins.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NetworkPlugins{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkPlugins) ValidateCreate() error {
	networkpluginslog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.

	if r.Spec.Plugins.NodeFeatureDiscovery != nil {
		klog.Info("ndf exist")

	}
	klog.Info("ndf not exist")
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkPlugins) ValidateUpdate(old runtime.Object) error {
	networkpluginslog.Info("validate update", "name", r.Name)

	client, err := getKubernetesClient()
	if err != nil {
		networkpluginslog.Error(err, "error creating kubernetes client")
		return err
	}

	networkpluginslog.Info("Fecthign Nad List")
	ctx := context.Background()
	var networkPluginsList = NetworkPluginsList{}
	if err := client.List(ctx, &networkPluginsList); err != nil {
		networkpluginslog.Info("Error fetching nad list")
		return err
	}

	networkpluginslog.Info("Checking multus")

	if err := r.multusUninstallCheck(client, networkPluginsList); err != nil {
		return err
	}

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkPlugins) ValidateDelete() error {
	networkpluginslog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

func (r *NetworkPlugins) multusUninstallCheck(client client.Client, networkPluginsList NetworkPluginsList) error {

	multusExist := false
	for _, networkPlugins := range networkPluginsList.Items {
		if networkPlugins.Spec.Plugins.Multus != nil {
			multusExist = true
		}
	}

	if multusExist && r.Spec.Plugins.Multus == nil {

		nadExist, err := r.networkAttachmentDefinitionExists(client)
		if err != nil {
			log.Printf("error checking for NetworkAttachmentDefinition: %v", err)
			return err
		}
		if nadExist {
			return fmt.Errorf("NetworkAttachmentDefinition exists on cluster multus cannot be removed without deleting the NetworkAttachmentDefinition first")
		}

	}

	return nil
}

func getKubernetesClient() (client.Client, error) {
	// Get the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	// Get the REST client from the client config
	restClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	return restClient, nil
}

// This function checks if NetworkAttachmentDefinition exists
func (r *NetworkPlugins) networkAttachmentDefinitionExists(client client.Client) (bool, error) {
	nadList := &nettypes.NetworkAttachmentDefinitionList{}
	err := client.List(context.TODO(), nadList)
	if k8sErrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, nad := range nadList.Items {
		networkpluginslog.Info("networkAttachmentDefinition exists", "name", nad.Name, "namespace", nad.Namespace)

	}
	return len(nadList.Items) != 0, nil
}

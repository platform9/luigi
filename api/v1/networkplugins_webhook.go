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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
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

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkPlugins) ValidateDelete() error {
	networkpluginslog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

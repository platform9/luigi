/*


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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NetworkPluginsSpec defines the desired state of NetworkPlugins
type NetworkPluginsSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	CniPlugins *CniPlugins `json:"cniPlugins,omitempty"`

	//HostNetworking *HostNetworking `json:hostNetworking,omitempty"`
}

type CniPlugins struct {
	Multus      *Multus      `json:"multus,omitempty"`
	Whereabouts *Whereabouts `json:"whereabouts,omitempty"`
	Sriov       *Sriov       `json:"sriov,omitempty"`
	NMState     *NMState     `json:"nmstate,omitempty"`
}

type Whereabouts struct {
	Namespace        string `json:"namespace,omitempty"`
	WhereaboutsImage string `json:"whereaboutsImage,omitempty"`
}

type Multus struct {
	Namespace   string `json:"namespace,omitempty"`
	MultusImage string `json:"multusImage,omitempty"`
}

type Sriov struct {
	Namespace      string `json:"namespace,omitempty"`
	SriovCniImage  string `json:"sriovCniImage,omitempty"`
	SriovDpImage   string `json:"sriovDpImage,omitempty"`
	SriovConfigMap string `json:"sriovConfigMap,omitempty"`
}

type NMState struct{}

// NetworkPluginsStatus defines the observed state of NetworkPlugins
type NetworkPluginsStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// NetworkPlugins is the Schema for the networkplugins API
type NetworkPlugins struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkPluginsSpec   `json:"spec,omitempty"`
	Status NetworkPluginsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkPluginsList contains a list of NetworkPlugins
type NetworkPluginsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPlugins `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkPlugins{}, &NetworkPluginsList{})
}

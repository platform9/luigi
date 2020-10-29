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

// HostNetworkConfigSpec defines the desired state of HostNetworkConfig
type HostNetworkConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	SriovConfig []SriovConfig `json:"sriovConfig,omitempty"`
}

type SriovConfig struct {
	PfName   *string `json:"pfName,omitempty"`
	PciAddr  *string `json:"pciAddr,omitempty"`
	VendorId *string `json:"vendorId,omitempty"`
	DeviceId *string `json:"deviceId,omitempty"`
	NumVfs   *int    `json:"numVfs,omitempty"`
	MTU      *int    `json:"mtu,omitempty"`
	VfDriver *string `json:"vfDriver,omitempty"`
	PfDriver *string `json:"pfDriver,omitempty"`
}

// HostNetworkConfigStatus defines the observed state of HostNetworkConfig
type HostNetworkConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// HostNetworkConfig is the Schema for the HostNetworkConfigs API
type HostNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostNetworkConfigSpec   `json:"spec,omitempty"`
	Status HostNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HostNetworkConfigList contains a list of HostNetworkConfig
type HostNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostNetworkConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostNetworkConfig{}, &HostNetworkConfigList{})
}

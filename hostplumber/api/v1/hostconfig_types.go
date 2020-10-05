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

// HostConfigSpec defines the desired state of HostConfig
type HostConfigSpec struct {
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

// HostConfigStatus defines the observed state of HostConfig
type HostConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// HostConfig is the Schema for the hostconfigs API
type HostConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostConfigSpec   `json:"spec,omitempty"`
	Status HostConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HostConfigList contains a list of HostConfig
type HostConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostConfig{}, &HostConfigList{})
}

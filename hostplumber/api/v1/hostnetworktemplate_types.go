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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HostNetworkTemplateSpec defines the desired state of HostNetworkTemplate
type HostNetworkTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	NodeSelector    map[string]string `json:"nodeSelector,omitempty"`
	InterfaceConfig []InterfaceConfig `json:"interfaceConfig,omitempty"`
	SriovConfig     []SriovConfig     `json:"sriovConfig,omitempty"`
	OvsConfig       []*OvsConfig      `json:"ovsConfig,omitempty"`
}

type InterfaceConfig struct {
	Name *string      `json:"name"`
	MTU  *int         `json:"mtu,omitempty"`
	IPv4 *IPv4Info    `json:"ipv4,omitempty"`
	IPv6 *IPv6Info    `json:"ipv6,omitempty"`
	Vlan []VlanConfig `json:"vlan,omitempty"`
}

type VlanConfig struct {
	VlanId *int    `json:"id"`
	Name   *string `json:"name,omitempty"`
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

type OvsConfig struct {
	NodeInterface string `json:"nodeInterface,omitempty"`
	BridgeName    string `json:"bridgeName,omitempty"`
}

// HostNetworkTemplateStatus defines the observed state of HostNetworkTemplate
type HostNetworkTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HostNetworkTemplate is the Schema for the hostnetworktemplates API
type HostNetworkTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostNetworkTemplateSpec   `json:"spec,omitempty"`
	Status HostNetworkTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HostNetworkTemplateList contains a list of HostNetworkTemplate
type HostNetworkTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostNetworkTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostNetworkTemplate{}, &HostNetworkTemplateList{})
}

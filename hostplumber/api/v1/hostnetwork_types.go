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

// HostNetworkSpec defines the desired state of HostNetwork
type HostNetworkSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// HostNetworkStatus defines the observed state of HostNetwork
type HostNetworkStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	OvsStatus       []*OvsStatus       `json:"ovsStatus,omitempty"`
	InterfaceStatus []*InterfaceStatus `json:"interfaceStatus,omitempty"`
	Routes          *Routes            `json:"routes,omitempty"`
	Sysctl          []string           `json:"sysctlConfig,omitempty"`
}

type OvsStatus struct {
	NodeInterface string `json:"nodeInterface,omitempty"`
	BridgeName    string `json:"bridgeName,omitempty"`
}

type InterfaceStatus struct {
	PfName       string       `json:"pfName,omitempty"`
	IPv4         *IPv4Info    `json:"ipv4,omitempty"`
	IPv6         *IPv6Info    `json:"ipv6,omitempty"`
	PciAddr      string       `json:"pciAddr,omitempty"`
	VendorId     string       `json:"vendorId,omitempty"`
	DeviceId     string       `json:"deviceId,omitempty"`
	MacAddr      string       `json:"mac,omitempty"`
	MTU          int          `json:"mtu,omitempty"`
	PfDriver     string       `json:"pfDriver,omitempty"`
	SriovEnabled bool         `json:"sriovEnabled"`
	SriovStatus  *SriovStatus `json:"sriovStatus,omitempty"`
}

type IPv4Info struct {
	Address []string `json:"address,omitempty"`
}

type IPv6Info struct {
	Address []string `json:"address,omitempty"`
}

type SriovStatus struct {
	TotalVfs int       `json:"totalVfs,omitempty"`
	NumVfs   int       `json:"numVfs,omitempty"`
	Vfs      []*VfInfo `json:"vfs,omitempty"`
}

type VfInfo struct {
	ID       int    `json:"id"`
	VfDriver string `json:"vfDriver"`
	PciAddr  string `json:"pciAddr"`
	Mac      string `json:"mac"`
	Vlan     int    `json:"vlan"`
	Qos      int    `json:"qos"`
	Spoofchk bool   `json:"spoofchk"`
	Trust    bool   `json:"trust"`
}

type Routes struct {
	V4Routes []*Route `json:"ipv4,omitempty"`
	V6Routes []*Route `json:"ipv6,omitempty"`
}

type Route struct {
	Dst string `json:"dst,omitempty"`
	Gw  string `json:"gw,omitempty"`
	Dev string `json:"dev,omitempty"`
	Src string `json:"src,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HostNetwork is the Schema for the hostnetworks API
type HostNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostNetworkSpec   `json:"spec,omitempty"`
	Status HostNetworkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HostNetworkList contains a list of HostNetwork
type HostNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostNetwork `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostNetwork{}, &HostNetworkList{})
}

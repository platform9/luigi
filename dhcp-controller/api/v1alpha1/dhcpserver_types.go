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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DHCPServerSpec defines the desired state of DHCPServer
type DHCPServerSpec struct {
	// Details of Networks
	// +kubebuilder:validation:Required
	Networks []Network `json:"networks,omitempty"`
}

type Network struct {
	// refers to net-attach-def to be served
	// +kubebuilder:validation:Required
	NetworkName string `json:"networkName,omitempty"`
	// refers to IP address to bind interface to
	// +kubebuilder:validation:Required
	InterfaceIp string `json:"interfaceIp,omitempty"`
	// refers to CIDR of server
	// +kubebuilder:validation:Required
	ServerCIDR CIDR `json:"cidr"`
	// refers to leasetime of IP
	// +kubebuilder:validation:Required
	LeaseTime string `json:"leaseTime,omitempty"`
	// refers to vlan
	VlanID string `json:"vlanid,omitempty"`
}

// CIDR defines CIDR of each network
type CIDR struct {
	// refers to cidr range
	// +kubebuilder:validation:Required
	CIDRIP string `json:"range"`
	// refers to start IP of range
	RangeStartIp string `json:"range_start,omitempty"`
	// refers to end IP of range
	RangeEndIp string `json:"range_end,omitempty"`
	// refers to gateway IP
	GwAddress string `json:"gateway,omitempty"`
}

// DHCPServerStatus defines the observed state of DHCPServer
type DHCPServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DHCPServer is the Schema for the dhcpservers API
type DHCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DHCPServerSpec   `json:"spec,omitempty"`
	Status DHCPServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DHCPServerList contains a list of DHCPServer
type DHCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DHCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DHCPServer{}, &DHCPServerList{})
}

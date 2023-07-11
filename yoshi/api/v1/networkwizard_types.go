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
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkWizardSpec defines the desired state of NetworkWizard
type NetworkWizardSpec struct {
	// Plugin specifies the type of network/CNI to create in the backend
	// Valid values for now: "calico" and "public", more as we add Multus networks
	Plugin string `json:"plugin,omitempty"`

	// CIDR notation for the network plugin to provision
	CIDR       *string `json:"cidr,omitempty"`
	RangeStart net.IP  `json:"range_start,omitempty"`
	RangeEnd   net.IP  `json:"range_end,omitempty"`

	// BGPConfiguration - valid only for "public" type networks
	BGPConfig *BGPConfig `json:"bgpConfig,omitempty"`
}

type BGPConfig struct {
	// Physical routers to peer with
	RemotePeers []BGPPeer `json:"peers,omitempty"`

	// ASN of peer router
	RemoteASN *uint32 `json:"remoteASN,omitempty"`

	// ASN to use for the cluster's BGP advertisements
	MyASN *uint32 `json:"myASN,omitempty"`
}

type BGPPeer struct {
	PeerIP *string `json:"peerIP"`

	// Adds a static route that may be needed to connect to a peer.
	ReachableBy *string `json:"reachableBy,omitempty"`
}

func (n NetworkWizard) ParseCIDR() (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(*n.Spec.CIDR)
}

// NetworkWizardStatus defines the observed state of NetworkWizard
type NetworkWizardStatus struct {
	// Bool indicating if the network was created by CNI plugin
	Created bool `json:"created,omitempty"`

	// Error message indicating CNI errors
	Reason string `json:"reason,omitempty"`

	// Stores Fixed IP Allocations for a given VM: <fixed IP> :< VM Name>
	IPAllocations map[string]string `json:"allocations,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NetworkWizard is the Schema for the networkwizards API
type NetworkWizard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkWizardSpec   `json:"spec,omitempty"`
	Status NetworkWizardStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkWizardList contains a list of NetworkWizard
type NetworkWizardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkWizard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkWizard{}, &NetworkWizardList{})
}

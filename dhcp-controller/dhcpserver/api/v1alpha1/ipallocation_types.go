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
	"net"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IPAllocationSpec defines the desired state of IPAllocation
type IPAllocationSpec struct {
	// Range is a string that represents an IP address
	Range string `json:"range"`
	// Allocations is the set of allocated IPs for the given range. Its` indices are a direct mapping to the
	// IP with the same index/offset for the allocation's range.
	Allocations map[string]IPAllocationOwner `json:"allocations"`
	// LeaseExpiry is the epoch time when the IP was set to expire in the leasefile
	LeaseExpiry string `json:"leaseExpiry"`
}

// ParseCIDR formats the Range of the IPAllocation
func (i IPAllocation) ParseCIDR() (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(i.Spec.Range)
}

// IPAllocationOwner represents metadata about the pod/container owner of a specific IP
type IPAllocationOwner struct {
	MacAddr   string `json:"macAddr,omitempty"`
	ObjectRef string `json:"objectRef,omitempty"`
}

// IPAllocationStatus defines the observed state of IPAllocation
type IPAllocationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// IPAllocation is the Schema for the ipallocations API
type IPAllocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPAllocationSpec   `json:"spec,omitempty"`
	Status IPAllocationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IPAllocationList contains a list of IPAllocation
type IPAllocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPAllocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPAllocation{}, &IPAllocationList{})
}

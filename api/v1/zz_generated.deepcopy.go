//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DhcpController) DeepCopyInto(out *DhcpController) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DhcpController.
func (in *DhcpController) DeepCopy() *DhcpController {
	if in == nil {
		return nil
	}
	out := new(DhcpController)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Dpdk) DeepCopyInto(out *Dpdk) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Dpdk.
func (in *Dpdk) DeepCopy() *Dpdk {
	if in == nil {
		return nil
	}
	out := new(Dpdk)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostPlumber) DeepCopyInto(out *HostPlumber) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostPlumber.
func (in *HostPlumber) DeepCopy() *HostPlumber {
	if in == nil {
		return nil
	}
	out := new(HostPlumber)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Multus) DeepCopyInto(out *Multus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Multus.
func (in *Multus) DeepCopy() *Multus {
	if in == nil {
		return nil
	}
	out := new(Multus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkPlugins) DeepCopyInto(out *NetworkPlugins) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkPlugins.
func (in *NetworkPlugins) DeepCopy() *NetworkPlugins {
	if in == nil {
		return nil
	}
	out := new(NetworkPlugins)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NetworkPlugins) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkPluginsList) DeepCopyInto(out *NetworkPluginsList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NetworkPlugins, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkPluginsList.
func (in *NetworkPluginsList) DeepCopy() *NetworkPluginsList {
	if in == nil {
		return nil
	}
	out := new(NetworkPluginsList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NetworkPluginsList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkPluginsSpec) DeepCopyInto(out *NetworkPluginsSpec) {
	*out = *in
	if in.Plugins != nil {
		in, out := &in.Plugins, &out.Plugins
		*out = new(Plugins)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkPluginsSpec.
func (in *NetworkPluginsSpec) DeepCopy() *NetworkPluginsSpec {
	if in == nil {
		return nil
	}
	out := new(NetworkPluginsSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkPluginsStatus) DeepCopyInto(out *NetworkPluginsStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkPluginsStatus.
func (in *NetworkPluginsStatus) DeepCopy() *NetworkPluginsStatus {
	if in == nil {
		return nil
	}
	out := new(NetworkPluginsStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeFeatureDiscovery) DeepCopyInto(out *NodeFeatureDiscovery) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeFeatureDiscovery.
func (in *NodeFeatureDiscovery) DeepCopy() *NodeFeatureDiscovery {
	if in == nil {
		return nil
	}
	out := new(NodeFeatureDiscovery)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Ovs) DeepCopyInto(out *Ovs) {
	*out = *in
	if in.DPDK != nil {
		in, out := &in.DPDK, &out.DPDK
		*out = new(Dpdk)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Ovs.
func (in *Ovs) DeepCopy() *Ovs {
	if in == nil {
		return nil
	}
	out := new(Ovs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Plugins) DeepCopyInto(out *Plugins) {
	*out = *in
	if in.Multus != nil {
		in, out := &in.Multus, &out.Multus
		*out = new(Multus)
		**out = **in
	}
	if in.Whereabouts != nil {
		in, out := &in.Whereabouts, &out.Whereabouts
		*out = new(Whereabouts)
		(*in).DeepCopyInto(*out)
	}
	if in.Sriov != nil {
		in, out := &in.Sriov, &out.Sriov
		*out = new(Sriov)
		**out = **in
	}
	if in.HostPlumber != nil {
		in, out := &in.HostPlumber, &out.HostPlumber
		*out = new(HostPlumber)
		**out = **in
	}
	if in.NodeFeatureDiscovery != nil {
		in, out := &in.NodeFeatureDiscovery, &out.NodeFeatureDiscovery
		*out = new(NodeFeatureDiscovery)
		**out = **in
	}
	if in.OVS != nil {
		in, out := &in.OVS, &out.OVS
		*out = new(Ovs)
		(*in).DeepCopyInto(*out)
	}
	if in.DhcpController != nil {
		in, out := &in.DhcpController, &out.DhcpController
		*out = new(DhcpController)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Plugins.
func (in *Plugins) DeepCopy() *Plugins {
	if in == nil {
		return nil
	}
	out := new(Plugins)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Sriov) DeepCopyInto(out *Sriov) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Sriov.
func (in *Sriov) DeepCopy() *Sriov {
	if in == nil {
		return nil
	}
	out := new(Sriov)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Whereabouts) DeepCopyInto(out *Whereabouts) {
	*out = *in
	if in.IpReconcilerNodeSelector != nil {
		in, out := &in.IpReconcilerNodeSelector, &out.IpReconcilerNodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Whereabouts.
func (in *Whereabouts) DeepCopy() *Whereabouts {
	if in == nil {
		return nil
	}
	out := new(Whereabouts)
	in.DeepCopyInto(out)
	return out
}

//go:build !ignore_autogenerated

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
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetwork) DeepCopyInto(out *HostNetwork) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetwork.
func (in *HostNetwork) DeepCopy() *HostNetwork {
	if in == nil {
		return nil
	}
	out := new(HostNetwork)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HostNetwork) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkList) DeepCopyInto(out *HostNetworkList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]HostNetwork, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkList.
func (in *HostNetworkList) DeepCopy() *HostNetworkList {
	if in == nil {
		return nil
	}
	out := new(HostNetworkList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HostNetworkList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkSpec) DeepCopyInto(out *HostNetworkSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkSpec.
func (in *HostNetworkSpec) DeepCopy() *HostNetworkSpec {
	if in == nil {
		return nil
	}
	out := new(HostNetworkSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkStatus) DeepCopyInto(out *HostNetworkStatus) {
	*out = *in
	if in.OvsStatus != nil {
		in, out := &in.OvsStatus, &out.OvsStatus
		*out = make([]*OvsStatus, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(OvsStatus)
				**out = **in
			}
		}
	}
	if in.InterfaceStatus != nil {
		in, out := &in.InterfaceStatus, &out.InterfaceStatus
		*out = make([]*InterfaceStatus, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(InterfaceStatus)
				(*in).DeepCopyInto(*out)
			}
		}
	}
	if in.Routes != nil {
		in, out := &in.Routes, &out.Routes
		*out = new(Routes)
		(*in).DeepCopyInto(*out)
	}
	if in.Sysctl != nil {
		in, out := &in.Sysctl, &out.Sysctl
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkStatus.
func (in *HostNetworkStatus) DeepCopy() *HostNetworkStatus {
	if in == nil {
		return nil
	}
	out := new(HostNetworkStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkTemplate) DeepCopyInto(out *HostNetworkTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkTemplate.
func (in *HostNetworkTemplate) DeepCopy() *HostNetworkTemplate {
	if in == nil {
		return nil
	}
	out := new(HostNetworkTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HostNetworkTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkTemplateList) DeepCopyInto(out *HostNetworkTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]HostNetworkTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkTemplateList.
func (in *HostNetworkTemplateList) DeepCopy() *HostNetworkTemplateList {
	if in == nil {
		return nil
	}
	out := new(HostNetworkTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HostNetworkTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkTemplateSpec) DeepCopyInto(out *HostNetworkTemplateSpec) {
	*out = *in
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.InterfaceConfig != nil {
		in, out := &in.InterfaceConfig, &out.InterfaceConfig
		*out = make([]InterfaceConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.SriovConfig != nil {
		in, out := &in.SriovConfig, &out.SriovConfig
		*out = make([]SriovConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.OvsConfig != nil {
		in, out := &in.OvsConfig, &out.OvsConfig
		*out = make([]*OvsConfig, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(OvsConfig)
				(*in).DeepCopyInto(*out)
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkTemplateSpec.
func (in *HostNetworkTemplateSpec) DeepCopy() *HostNetworkTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(HostNetworkTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostNetworkTemplateStatus) DeepCopyInto(out *HostNetworkTemplateStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostNetworkTemplateStatus.
func (in *HostNetworkTemplateStatus) DeepCopy() *HostNetworkTemplateStatus {
	if in == nil {
		return nil
	}
	out := new(HostNetworkTemplateStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IPv4Info) DeepCopyInto(out *IPv4Info) {
	*out = *in
	if in.Address != nil {
		in, out := &in.Address, &out.Address
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IPv4Info.
func (in *IPv4Info) DeepCopy() *IPv4Info {
	if in == nil {
		return nil
	}
	out := new(IPv4Info)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IPv6Info) DeepCopyInto(out *IPv6Info) {
	*out = *in
	if in.Address != nil {
		in, out := &in.Address, &out.Address
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IPv6Info.
func (in *IPv6Info) DeepCopy() *IPv6Info {
	if in == nil {
		return nil
	}
	out := new(IPv6Info)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InterfaceConfig) DeepCopyInto(out *InterfaceConfig) {
	*out = *in
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
	if in.MTU != nil {
		in, out := &in.MTU, &out.MTU
		*out = new(int)
		**out = **in
	}
	if in.IPv4 != nil {
		in, out := &in.IPv4, &out.IPv4
		*out = new(IPv4Info)
		(*in).DeepCopyInto(*out)
	}
	if in.IPv6 != nil {
		in, out := &in.IPv6, &out.IPv6
		*out = new(IPv6Info)
		(*in).DeepCopyInto(*out)
	}
	if in.Vlan != nil {
		in, out := &in.Vlan, &out.Vlan
		*out = make([]VlanConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InterfaceConfig.
func (in *InterfaceConfig) DeepCopy() *InterfaceConfig {
	if in == nil {
		return nil
	}
	out := new(InterfaceConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InterfaceStatus) DeepCopyInto(out *InterfaceStatus) {
	*out = *in
	if in.IPv4 != nil {
		in, out := &in.IPv4, &out.IPv4
		*out = new(IPv4Info)
		(*in).DeepCopyInto(*out)
	}
	if in.IPv6 != nil {
		in, out := &in.IPv6, &out.IPv6
		*out = new(IPv6Info)
		(*in).DeepCopyInto(*out)
	}
	if in.SriovStatus != nil {
		in, out := &in.SriovStatus, &out.SriovStatus
		*out = new(SriovStatus)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InterfaceStatus.
func (in *InterfaceStatus) DeepCopy() *InterfaceStatus {
	if in == nil {
		return nil
	}
	out := new(InterfaceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OvsConfig) DeepCopyInto(out *OvsConfig) {
	*out = *in
	if in.Params != nil {
		in, out := &in.Params, &out.Params
		*out = new(Params)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OvsConfig.
func (in *OvsConfig) DeepCopy() *OvsConfig {
	if in == nil {
		return nil
	}
	out := new(OvsConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OvsStatus) DeepCopyInto(out *OvsStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OvsStatus.
func (in *OvsStatus) DeepCopy() *OvsStatus {
	if in == nil {
		return nil
	}
	out := new(OvsStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Params) DeepCopyInto(out *Params) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Params.
func (in *Params) DeepCopy() *Params {
	if in == nil {
		return nil
	}
	out := new(Params)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Route) DeepCopyInto(out *Route) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Route.
func (in *Route) DeepCopy() *Route {
	if in == nil {
		return nil
	}
	out := new(Route)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Routes) DeepCopyInto(out *Routes) {
	*out = *in
	if in.V4Routes != nil {
		in, out := &in.V4Routes, &out.V4Routes
		*out = make([]*Route, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(Route)
				**out = **in
			}
		}
	}
	if in.V6Routes != nil {
		in, out := &in.V6Routes, &out.V6Routes
		*out = make([]*Route, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(Route)
				**out = **in
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Routes.
func (in *Routes) DeepCopy() *Routes {
	if in == nil {
		return nil
	}
	out := new(Routes)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SriovConfig) DeepCopyInto(out *SriovConfig) {
	*out = *in
	if in.PfName != nil {
		in, out := &in.PfName, &out.PfName
		*out = new(string)
		**out = **in
	}
	if in.PciAddr != nil {
		in, out := &in.PciAddr, &out.PciAddr
		*out = new(string)
		**out = **in
	}
	if in.VendorId != nil {
		in, out := &in.VendorId, &out.VendorId
		*out = new(string)
		**out = **in
	}
	if in.DeviceId != nil {
		in, out := &in.DeviceId, &out.DeviceId
		*out = new(string)
		**out = **in
	}
	if in.NumVfs != nil {
		in, out := &in.NumVfs, &out.NumVfs
		*out = new(int)
		**out = **in
	}
	if in.MTU != nil {
		in, out := &in.MTU, &out.MTU
		*out = new(int)
		**out = **in
	}
	if in.VfDriver != nil {
		in, out := &in.VfDriver, &out.VfDriver
		*out = new(string)
		**out = **in
	}
	if in.PfDriver != nil {
		in, out := &in.PfDriver, &out.PfDriver
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SriovConfig.
func (in *SriovConfig) DeepCopy() *SriovConfig {
	if in == nil {
		return nil
	}
	out := new(SriovConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SriovStatus) DeepCopyInto(out *SriovStatus) {
	*out = *in
	if in.Vfs != nil {
		in, out := &in.Vfs, &out.Vfs
		*out = make([]*VfInfo, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(VfInfo)
				**out = **in
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SriovStatus.
func (in *SriovStatus) DeepCopy() *SriovStatus {
	if in == nil {
		return nil
	}
	out := new(SriovStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VfInfo) DeepCopyInto(out *VfInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VfInfo.
func (in *VfInfo) DeepCopy() *VfInfo {
	if in == nil {
		return nil
	}
	out := new(VfInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VlanConfig) DeepCopyInto(out *VlanConfig) {
	*out = *in
	if in.VlanId != nil {
		in, out := &in.VlanId, &out.VlanId
		*out = new(int)
		**out = **in
	}
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VlanConfig.
func (in *VlanConfig) DeepCopy() *VlanConfig {
	if in == nil {
		return nil
	}
	out := new(VlanConfig)
	in.DeepCopyInto(out)
	return out
}

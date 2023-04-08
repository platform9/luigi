package vmutils

import (
	"fmt"

	"github.com/platform9/luigi/yoshi/pkg/cni"
	"github.com/platform9/luigi/yoshi/pkg/utils/constants"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func GetVMNetworkAnnotation(vm *kubevirtv1.VirtualMachine) string {
	if vm.Annotations == nil {
		return ""
	}
	net, ok := vm.Annotations[constants.Pf9NetworkAnnotation]
	if !ok {
		return ""
	}
	return net
}

func VMHasFixedIP(vm *kubevirtv1.VirtualMachine) bool {
	if vm.Spec.Template.ObjectMeta.Annotations == nil {
		return false
	}

	ip, ok := vm.Spec.Template.ObjectMeta.Annotations[cni.CalicoFixedIpAnnotation]
	if !ok || ip == "" {
		return false
	}
	return true
}

func SetVMFixedIP(vm *kubevirtv1.VirtualMachine, fixedIP string) {
	if vm.Spec.Template.ObjectMeta.Annotations == nil {
		vm.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	ipAnnotation := "[\"" + fixedIP + "\"]"
	vm.Spec.Template.ObjectMeta.Annotations[cni.CalicoFixedIpAnnotation] = ipAnnotation
}

func SetVMIServiceName(vm *kubevirtv1.VirtualMachine) {
	if vm.Spec.Template.ObjectMeta.Labels == nil {
		vm.Spec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	// Label the VMI so controller can create a service later for Public IP
	// The label must be on VMI, not VM
	vm.Spec.Template.ObjectMeta.Labels[constants.Pf9VMIServiceLabel] = GetVmRef(vm)
}

func GetVMPublicIP(vm *kubevirtv1.VirtualMachine) string {
	if vm.Annotations == nil {
		return ""
	}

	ip, ok := vm.Annotations[constants.Pf9PublicIPAnnotation]
	if !ok {
		return ""
	}

	return ip
}

func VMHasStaticMAC(vm *kubevirtv1.VirtualMachine) bool {
	if vm.Spec.Template.ObjectMeta.Annotations == nil {
		return false
	}

	MAC, ok := vm.Spec.Template.ObjectMeta.Annotations[cni.CalicoMACAnnotation]
	if !ok || MAC == "" {
		return false
	}
	return true
}

func SetVMStaticMAC(vm *kubevirtv1.VirtualMachine, MAC string) {
	if vm.Spec.Template.ObjectMeta.Annotations == nil {
		vm.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	vm.Spec.Template.ObjectMeta.Annotations[cni.CalicoMACAnnotation] = MAC
}

func GetVmRef(vm *kubevirtv1.VirtualMachine) string {
	return fmt.Sprintf("%s.%s", vm.GetName(), vm.GetNamespace())
}

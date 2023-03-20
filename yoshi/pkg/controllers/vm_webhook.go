package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/platform9/luigi/yoshi/pkg/utils/vmutils"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-v1-vm,mutating=true,failurePolicy=fail,sideEffects=None,groups="kubevirt.io",resources=virtualmachines,verbs=create;update,versions=v1,name=mvm.kb.io,admissionReviewVersions=v1

func (a *VMAnnotator) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}

type VMAnnotator struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (a *VMAnnotator) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := logf.FromContext(ctx)
	vm := &kubevirtv1.VirtualMachine{}
	err := a.decoder.Decode(req, vm)
	if err != nil {
		log.Info("ERROR deocding VM object regular")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if vm.Annotations == nil {
		// No VM Network annotation, skip and deploy like normal
		log.Info("VM has no reserved network", "VM", vm.Name)
		return ReturnPatchedVM(vm, req)
	}

	netName := vmutils.GetVMNetworkAnnotation(vm)
	if netName == "" {
		return admission.Allowed("No VM network reservation")
	}

	if vmutils.VMHasFixedIP(vm) {
		return admission.Allowed("VM request has fixed IP")
	}

	// VM controller will start the VM after allocating Fixed IP
	stopped := false
	vm.Spec.Running = &stopped
	vmutils.SetVMIServiceName(vm)

	return ReturnPatchedVM(vm, req)
}

func ReturnPatchedVM(vm *kubevirtv1.VirtualMachine, req admission.Request) admission.Response {
	marshaledVM, err := json.Marshal(vm)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledVM)
}

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	plumberv1 "github.com/platform9/luigi/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-v1-networkplugins,mutating=true,failurePolicy=fail,sideEffects=None,groups="plumber.k8s.pf9.io",resources=networkplugins,verbs=get;list;watch;create;update;patch;delete,versions=v1,admissionReviewVersions=v1,name=np.plumber.io

func (a *NetworkPluginsValidator) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}

type NetworkPluginsValidator struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (a *NetworkPluginsValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	return admission.Errored(http.StatusInternalServerError, fmt.Errorf("Testing"))
	// log := logf.FromContext(ctx)
	// np := &plumberv1.NetworkPlugins{}
	// err := a.decoder.Decode(req, np)
	// if err != nil {
	// 	log.Info("ERROR deocding VM object regular")
	// 	return admission.Errored(http.StatusBadRequest, err)
	// }
	// log.Info("<> Webhook Network Plugin", "NetworkPlugins Req", np)

	// return ReturnPatchedVM(np, req)
}

func ReturnPatchedVM(np *plumberv1.NetworkPlugins, req admission.Request) admission.Response {
	marshaledNP, err := json.Marshal(np)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledNP)
}

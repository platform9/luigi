package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	plumberv1 "github.com/platform9/luigi/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

	log := logf.FromContext(ctx)
	networkPluginsReq := &plumberv1.NetworkPlugins{}
	err := a.decoder.Decode(req, networkPluginsReq)
	if err != nil {
		log.Error(err, "Error decoding NetworkPlugins")
		return admission.Errored(http.StatusBadRequest, err)
	}

	var networkPluginsList = &plumberv1.NetworkPluginsList{}
	if err := a.Client.List(ctx, networkPluginsList); err != nil {
		log.Error(err, "Error listing NetworkPluginsList")
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error listing NetworkPluginsList: %w", err))
	}

	multusExist := false
	for _, networkPlugins := range networkPluginsList.Items {
		if networkPlugins.Spec.Plugins.Multus != nil {
			multusExist = true
		}
	}

	if multusExist && (networkPluginsReq.Spec.Plugins == nil || networkPluginsReq.Spec.Plugins.Multus == nil) {
		nadExist, err := a.networkAttachmentDefinitionExists(a.Client)
		if err != nil {
			log.Error(err, "Error checking for NetworkAttachmentDefinition")
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error while fetching network attachment definition: %v", err))
		}
		if nadExist {
			return admission.Denied("NetworkAttachmentDefinition exists on cluster. multus cannot be removed without deleting the NetworkAttachmentDefinition first")
		}
	}

	return ReturnPatchedNetworkPlugins(networkPluginsReq, req)
}

func ReturnPatchedNetworkPlugins(networkPluginsReq *plumberv1.NetworkPlugins, req admission.Request) admission.Response {
	marshaledNetworkPlugins, err := json.Marshal(networkPluginsReq)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledNetworkPlugins)
}

// This function checks if NetworkAttachmentDefinition exists
func (a *NetworkPluginsValidator) networkAttachmentDefinitionExists(client client.Client) (bool, error) {
	nadList := &nettypes.NetworkAttachmentDefinitionList{}
	err := client.List(context.TODO(), nadList)
	if err != nil {
		return false, err
	}
	return len(nadList.Items) != 0, nil
}

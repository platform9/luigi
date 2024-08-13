package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	plumberv1 "github.com/platform9/luigi/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	var networkPluginsList = &plumberv1.NetworkPluginsList{}
	err := a.Client.List(ctx, networkPluginsList)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Error listing NetworkPluginsList")
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error listing NetworkPluginsList: %w", err))
	}

	if req.Operation == admissionv1.Create {
		if len(networkPluginsList.Items) != 0 {
			err = fmt.Errorf("NetworkPlugins already exists, only one NetworkPlugins can be installed")
			log.Info(err.Error())
			return admission.Denied(err.Error())
		}
	}

	multusExist := false
	for _, networkPlugins := range networkPluginsList.Items {
		if networkPlugins.Spec.Plugins != nil && networkPlugins.Spec.Plugins.Multus != nil {
			multusExist = true
		}
	}

	if req.Operation == admissionv1.Delete && multusExist {
		if _, err := a.multusUninstallCheck(); err != nil {
			log.Error(err, "error while doing checks for multus uninstall")
			return admission.Denied("NetworkPlugins cannot be deleted. Please delete the NetworkAttachmentDefinition first")
		}
		return admission.Allowed("Delete request")
	}

	networkPluginsReq := &plumberv1.NetworkPlugins{}
	log.Info("DEBUGTEST", "request", req)
	err = a.decoder.Decode(req, networkPluginsReq)
	if err != nil {
		log.Error(nil, "request: ", req)
		log.Error(err, "Error decoding NetworkPlugins")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if multusExist && (networkPluginsReq.Spec.Plugins == nil || networkPluginsReq.Spec.Plugins.Multus == nil) {
		if resp, err := a.multusUninstallCheck(); err != nil {
			log.Error(err, "error while doing checks for multus uninstall")
			return resp
		}

	}

	if err := a.isNetworkPluginsValid(networkPluginsReq, networkPluginsList); err != nil {
		log.Error(err, "NetworkPlugins already exist, New can not be installed before removing old ones")
		return admission.Denied(fmt.Sprintf("NetworkPlugins already exists: %v", err.Error()))
	}

	return ReturnPatchedNetworkPlugins(networkPluginsReq, req)
}

func (a *NetworkPluginsValidator) multusUninstallCheck() (admission.Response, error) {
	nadExist, err := a.networkAttachmentDefinitionExists(a.Client)
	if err != nil {
		err = fmt.Errorf("error while fetching network attachment definition: %v", err)
		return admission.Errored(http.StatusInternalServerError, err), err
	}
	if nadExist {
		err = fmt.Errorf("NetworkAttachmentDefinition exists on cluster. multus cannot be removed without deleting the NetworkAttachmentDefinition first")
		return admission.Denied(err.Error()), err
	}
	return admission.Response{}, nil

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

func (a *NetworkPluginsValidator) isNetworkPluginsValid(networkPluginsReq *plumberv1.NetworkPlugins, networkPluginsList *plumberv1.NetworkPluginsList) error {
	reqPlugins := networkPluginsReq.Spec.Plugins

	for _, networkPlugins := range networkPluginsList.Items {
		if reqPlugins.Multus != nil && networkPlugins.Spec.Plugins.Multus != nil {
			if reqPlugins.Multus.Namespace != networkPlugins.Spec.Plugins.Multus.Namespace {
				return fmt.Errorf(" Multus already exists on cluster, remove it and reinstall ")
			}
		}

		if reqPlugins.Whereabouts != nil && networkPlugins.Spec.Plugins.Whereabouts != nil {
			if reqPlugins.Whereabouts.Namespace != networkPlugins.Spec.Plugins.Whereabouts.Namespace {
				return fmt.Errorf(" Whereabouts already exists on cluster, remove it and reinstall ")
			}

		}

		if reqPlugins.Sriov != nil && networkPlugins.Spec.Plugins.Sriov != nil {
			if reqPlugins.Sriov.Namespace != networkPlugins.Spec.Plugins.Sriov.Namespace {
				return fmt.Errorf(" Sriov already exists on cluster, remove it and reinstall ")
			}

		}

		if reqPlugins.HostPlumber != nil && networkPlugins.Spec.Plugins.HostPlumber != nil {
			if reqPlugins.HostPlumber.Namespace != networkPlugins.Spec.Plugins.HostPlumber.Namespace {
				return fmt.Errorf("HostPlumber already exists on cluster, remove it and reinstall ")
			}

		}

		if reqPlugins.NodeFeatureDiscovery != nil && networkPlugins.Spec.Plugins.NodeFeatureDiscovery != nil {
			if reqPlugins.NodeFeatureDiscovery.Namespace != networkPlugins.Spec.Plugins.NodeFeatureDiscovery.Namespace {
				return fmt.Errorf(" NodeFeatureDiscovery already exists on cluster, remove it and reinstall")
			}

		}

		if reqPlugins.OVS != nil && networkPlugins.Spec.Plugins.OVS != nil {
			if reqPlugins.OVS.Namespace != networkPlugins.Spec.Plugins.OVS.Namespace {
				return fmt.Errorf(" OVS already exists on cluster, remove it and reinstall")
			}

		}

		if reqPlugins.DhcpController != nil && networkPlugins.Spec.Plugins.DhcpController != nil {
			if reqPlugins.DhcpController.KubemacpoolNamespace != networkPlugins.Spec.Plugins.DhcpController.KubemacpoolNamespace {
				return fmt.Errorf(" DhcpController already exists on cluster, remove it and reinstall ")
			}
		}
	}

	return nil
}

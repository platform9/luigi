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

package controllers

import (
	"bufio"
	"bytes"
	"context"

	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"github.com/dustin/go-humanize"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	plumberv1 "github.com/platform9/luigi/api/v1"
	"github.com/platform9/luigi/pkg/apply"
)

const (
	DefaultNamespace        = "luigi-system"
	KubemacpoolNamespace    = "dhcp-controller-system"
	MultusImage             = "docker.io/platform9/multus:v3.7.2-pmk-2644970"
	WhereaboutsImage        = "docker.io/platform9/whereabouts:v0.6-pmk-2871011"
	SriovCniImage           = "docker.io/platform9/sriov-cni:v2.6.2-pmk-2571007"
	SriovDpImage            = "docker.io/platform9/sriov-network-device-plugin:v3.3.2-pmk-2571186"
	OvsImage                = "docker.io/platform9/openvswitch:v2.17.5-1"
	OvsCniImage             = "quay.io/kubevirt/ovs-cni-plugin:v0.28.0"
	OvsMarkerImage          = "quay.io/kubevirt/ovs-cni-marker:v0.28.0"
	HostPlumberImage        = "docker.io/platform9/hostplumber:v0.5.2"
	DhcpControllerImage     = "docker.io/platform9/pf9-dhcp-controller:v1.0"
	KubemacpoolImage        = "quay.io/kubevirt/kubemacpool:v0.41.0"
	KubemacpoolRangeStart   = "02:55:43:00:00:00"
	KubemacpoolRangeEnd     = "02:55:43:FF:FF:FF"
	KubeRbacProxyImage      = "gcr.io/kubebuilder/kube-rbac-proxy:v0.8.0"
	NfdImage                = "docker.io/platform9/node-feature-discovery:v0.11.3-pmk-2575337"
	TemplateDir             = "/etc/plugin_templates/"
	CreateDir               = TemplateDir + "create/"
	DeleteDir               = TemplateDir + "delete/"
	NetworkPluginsConfigMap = "pf9-networkplugins-config"
	IpReconcilerSchedule    = "*/5 * * * *"
	HugepageSize            = "2Mi"
)

// NetworkPluginsReconciler reconciles a NetworkPlugins object
type NetworkPluginsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

type PluginsUpdateInfo struct {
	Log            logr.Logger
	NamespacedName types.NamespacedName
	currentSpec    *plumberv1.NetworkPluginsSpec
	prevSpec       *plumberv1.NetworkPluginsSpec
}

type MultusT plumberv1.Multus
type WhereaboutsT plumberv1.Whereabouts
type SriovT plumberv1.Sriov
type OvsT plumberv1.Ovs
type HostPlumberT plumberv1.HostPlumber
type DhcpControllerT plumberv1.DhcpController
type NodeFeatureDiscoveryT plumberv1.NodeFeatureDiscovery

type ApplyPlugin interface {
	WriteConfigToTemplate(string, string) error
	ApplyTemplate(string) error
}

//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkplugins,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkplugins/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkplugins/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkPlugins object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *NetworkPluginsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := r.Log.WithValues("networkplugins", req.NamespacedName)

	var networkPluginsReq = plumberv1.NetworkPlugins{}
	if err := r.Get(ctx, req.NamespacedName, &networkPluginsReq); err != nil {
		log.Error(err, "unable to fetch NetworkPlugins")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var reqInfo *PluginsUpdateInfo = new(PluginsUpdateInfo)
	reqInfo.Log = log
	reqInfo.NamespacedName = req.NamespacedName
	reqInfo.prevSpec = nil
	reqInfo.currentSpec = &networkPluginsReq.Spec
	cm, err := r.getCurrentConfig(ctx, req)
	if err != nil {
		// TODO - Error fetching previous config (NOT a not found error) - ignore?
		log.Info("Error fetching previous ConfigMap, ignoring...")
	}
	if cm != nil {
		reqInfo.prevSpec, err = convertConfigMapToSpec(cm)
		if err != nil {
			log.Error(err, "Error converting previous ConfigMap to Spec")
			return ctrl.Result{}, err
		}
	}

	pluginsFinalizerName := "teardownPlugins"

	if networkPluginsReq.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(networkPluginsReq.GetFinalizers(), pluginsFinalizerName) {
			controllerutil.AddFinalizer(&networkPluginsReq, pluginsFinalizerName)
			if err := r.Update(ctx, &networkPluginsReq); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if containsString(networkPluginsReq.GetFinalizers(), pluginsFinalizerName) {
			if err := r.TeardownPlugins(reqInfo); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&networkPluginsReq, pluginsFinalizerName)
			if err := r.Update(ctx, &networkPluginsReq); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	var fileList []string
	err = r.parseNewPlugins(reqInfo, &fileList)
	if err != nil {
		log.Error(err, "Error applying new plugin templates")
		return ctrl.Result{}, err
	}
	log.Info("Applying plugin manifests: ", "fileList", fileList)
	err = r.createPlugins(fileList)
	if err != nil {
		return ctrl.Result{}, err
	}

	var fileListMissing []string
	err = r.parseMissingPlugins(reqInfo, &fileListMissing)
	if err != nil {
		log.Error(err, "Error applying templates!")
		return ctrl.Result{}, err
	}
	log.Info("Deleting plugin manifests", "fileListMissing", fileListMissing)
	err = r.deleteMissingPlugins(fileListMissing)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Everything succeeded - save Spec we just applied to ConfigMap
	err = r.saveSpecConfig(ctx, reqInfo)
	if err != nil {
		log.Error(err, "Failed to save spec in new ConfigMap")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (hostPlumberConfig *HostPlumberT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})
	if hostPlumberConfig.Namespace != "" {
		config["Namespace"] = hostPlumberConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if hostPlumberConfig.ImagePullPolicy == "Always" {
		config["ImagePullPolicy"] = "Always"
	} else {
		config["ImagePullPolicy"] = "IfNotPresent"
	}

	if hostPlumberConfig.HostPlumberImage != "" {
		config["HostPlumberImage"] = hostPlumberConfig.HostPlumberImage
	} else {
		config["HostPlumberImage"] = ReplaceContainerRegistry(HostPlumberImage, registry)
	}

	config["KubeRbacProxyImage"] = ReplaceContainerRegistry(KubeRbacProxyImage, registry)

	t, err := template.ParseFiles(filepath.Join(TemplateDir, "pf9-hostplumber", "hostplumber.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "hostplumber.yaml")); err != nil {
		return err
	}
	return nil
}

func (hostPlumberConfig *HostPlumberT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying PF9 Host Plumber\n")
	return nil
}

func (dhcpControllerConfig *DhcpControllerT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})

	if dhcpControllerConfig.ImagePullPolicy == "Always" {
		config["ImagePullPolicy"] = "Always"
	} else {
		config["ImagePullPolicy"] = "IfNotPresent"
	}

	if dhcpControllerConfig.DhcpControllerImage != "" {
		config["DhcpControllerImage"] = dhcpControllerConfig.DhcpControllerImage
	} else {
		config["DhcpControllerImage"] = ReplaceContainerRegistry(DhcpControllerImage, registry)
	}

	if dhcpControllerConfig.KubemacpoolNamespace != "" {
		config["KubemacpoolNamespace"] = dhcpControllerConfig.KubemacpoolNamespace
	} else {
		config["KubemacpoolNamespace"] = KubemacpoolNamespace
	}

	if dhcpControllerConfig.KubemacpoolRangeStart != "" {
		config["KubemacpoolRangeStart"] = dhcpControllerConfig.KubemacpoolRangeStart
	} else {
		config["KubemacpoolRangeStart"] = KubemacpoolRangeStart
	}

	if dhcpControllerConfig.KubemacpoolRangeEnd != "" {
		config["KubemacpoolRangeEnd"] = dhcpControllerConfig.KubemacpoolRangeEnd
	} else {
		config["KubemacpoolRangeEnd"] = KubemacpoolRangeEnd
	}

	config["KubeRbacProxyImage"] = ReplaceContainerRegistry(KubeRbacProxyImage, registry)
	config["KubemacpoolImage"] = ReplaceContainerRegistry(KubemacpoolImage, registry)

	t, err := template.ParseFiles(filepath.Join(TemplateDir, "dhcpcontroller", "dhcpcontroller.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "dhcpcontroller.yaml")); err != nil {
		return err
	}
	return nil
}

func (dhcpControllerConfig *DhcpControllerT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying PF9 dhcp-controller\n")
	return nil
}

func (nfdConfig *NodeFeatureDiscoveryT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})

	if nfdConfig.NfdImage != "" {
		config["NfdImage"] = nfdConfig.NfdImage
	} else {
		config["NfdImage"] = ReplaceContainerRegistry(NfdImage, registry)
	}

	if nfdConfig.ImagePullPolicy == "Always" {
		config["ImagePullPolicy"] = "Always"
	} else {
		config["ImagePullPolicy"] = "IfNotPresent"
	}

	t, err := template.ParseFiles(filepath.Join(TemplateDir, "node-feature-discovery", "nfd.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "nfd.yaml")); err != nil {
		return err
	}
	return nil
}

func (nfdConfig *NodeFeatureDiscoveryT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying Node Feature Discovery\n")
	return nil
}

func (multusConfig *MultusT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})
	if multusConfig.Namespace != "" {
		config["Namespace"] = multusConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if multusConfig.ImagePullPolicy == "Always" {
		config["ImagePullPolicy"] = "Always"
	} else {
		config["ImagePullPolicy"] = "IfNotPresent"
	}

	if multusConfig.MultusImage != "" {
		config["MultusImage"] = multusConfig.MultusImage
	} else {
		config["MultusImage"] = ReplaceContainerRegistry(MultusImage, registry)
	}

	t, err := template.ParseFiles(filepath.Join(TemplateDir, "multus", "multus.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "multus.yaml")); err != nil {
		return err
	}
	return nil
}

func (multusConfig *MultusT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying Multus\n")
	return nil
}

func (whereaboutsConfig *WhereaboutsT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})
	if whereaboutsConfig.Namespace != "" {
		config["Namespace"] = whereaboutsConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if whereaboutsConfig.ImagePullPolicy == "Always" {
		config["ImagePullPolicy"] = "Always"
	} else {
		config["ImagePullPolicy"] = "IfNotPresent"
	}

	if whereaboutsConfig.WhereaboutsImage != "" {
		config["WhereaboutsImage"] = whereaboutsConfig.WhereaboutsImage
	} else {
		config["WhereaboutsImage"] = ReplaceContainerRegistry(WhereaboutsImage, registry)
	}

	if whereaboutsConfig.IpReconcilerSchedule != "" {
		config["IpReconcilerSchedule"] = whereaboutsConfig.IpReconcilerSchedule
	} else {
		config["IpReconcilerSchedule"] = IpReconcilerSchedule
	}

	if whereaboutsConfig.IpReconcilerNodeSelector != nil {
		config["NodeSelector"] = whereaboutsConfig.IpReconcilerNodeSelector
	}

	t, err := template.ParseFiles(filepath.Join(TemplateDir, "whereabouts", "whereabouts.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "whereabouts.yaml")); err != nil {
		return err
	}
	return nil
}

func (whereaboutsConfig *WhereaboutsT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying Whereabouts\n")
	return nil
}

func (sriovConfig *SriovT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})

	if sriovConfig.Namespace != "" {
		config["Namespace"] = sriovConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if sriovConfig.SriovCniImage != "" {
		config["SriovCniImage"] = sriovConfig.SriovCniImage
	} else {
		config["SriovCniImage"] = ReplaceContainerRegistry(SriovCniImage, registry)
	}

	if sriovConfig.SriovDpImage != "" {
		config["SriovDpImage"] = sriovConfig.SriovDpImage
	} else {
		config["SriovDpImage"] = ReplaceContainerRegistry(SriovDpImage, registry)
	}

	// Apply the SRIOV CNI
	t, err := template.ParseFiles(filepath.Join(TemplateDir, "sriov", "sriov-cni.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, outputDir+"sriov-cni.yaml"); err != nil {
		return err
	}

	// Apply the SRIOV Device Plugin
	t, err = template.ParseFiles(filepath.Join(TemplateDir, "sriov", "sriov-deviceplugin.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "sriov-deviceplugin.yaml")); err != nil {
		return err
	}
	return nil
}

func (sriovConfig *SriovT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying SRIOV")
	return nil
}

func (ovsConfig *OvsT) WriteConfigToTemplate(outputDir, registry string) error {
	config := make(map[string]interface{})

	if ovsConfig.Namespace != "" {
		config["Namespace"] = ovsConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if ovsConfig.ImagePullPolicy == "Always" {
		config["ImagePullPolicy"] = "Always"
	} else {
		config["ImagePullPolicy"] = "IfNotPresent"
	}

	if ovsConfig.OVSImage != "" {
		config["OVSImage"] = ovsConfig.OVSImage
	} else {
		config["OVSImage"] = ReplaceContainerRegistry(OvsImage, registry)
	}

	if ovsConfig.CNIImage != "" {
		config["CNIImage"] = ovsConfig.CNIImage
	} else {
		config["CNIImage"] = ReplaceContainerRegistry(OvsCniImage, registry)
	}

	if ovsConfig.MarkerImage != "" {
		config["MarkerImage"] = ovsConfig.MarkerImage
	} else {
		config["MarkerImage"] = ReplaceContainerRegistry(OvsMarkerImage, registry)
	}

	if ovsConfig.DPDK != nil {
		if ovsConfig.DPDK.LcoreMask == "" || ovsConfig.DPDK.SocketMem == "" || ovsConfig.DPDK.PmdCpuMask == "" || ovsConfig.DPDK.HugepageMemory == "" {
			return fmt.Errorf("LcoreMask, SocketMem, PmdCpuMask, HugepageMemory are required parameters to enable Dpdk")
		}
		config["HugepageSize"] = GetHugepageSize()
		config["DPDK"] = ovsConfig.DPDK
	}

	// Apply the OVS DaemonSet
	t, err := template.ParseFiles(filepath.Join(TemplateDir, "ovs", "ovs-daemons.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "ovs-daemons.yaml")); err != nil {
		return err
	}

	// Apply the OVS CNI
	t, err = template.ParseFiles(filepath.Join(TemplateDir, "ovs", "ovs-cni.yaml"))
	if err != nil {
		return err
	}

	if err := renderTemplateToFile(config, t, filepath.Join(outputDir, "ovs-cni.yaml")); err != nil {
		return err
	}

	return nil
}

func (ovsConfig *OvsT) ApplyTemplate(outputDir string) error {
	fmt.Printf("Applying Ovs\n")
	return nil
}

func ReplaceContainerRegistry(originalImage, newRegistry string) string {
	if newRegistry == "" {
		return originalImage
	}

	r1 := regexp.MustCompile(`^[^\/]*`)
	privateImg := r1.ReplaceAllString(originalImage, newRegistry)
	return privateImg
}

func (r *NetworkPluginsReconciler) createPlugin(plugin ApplyPlugin, registry string) error {
	outputDir := CreateDir

	if err := plugin.WriteConfigToTemplate(outputDir, registry); err != nil {
		fmt.Printf("WriteConfigToTemplate returned error: %s\n", err)
		return err
	}

	if err := plugin.ApplyTemplate(outputDir); err != nil {
		return err
	}

	return nil
}

func (r *NetworkPluginsReconciler) deletePlugin(plugin ApplyPlugin, registry string) error {
	outputDir := DeleteDir

	if err := plugin.WriteConfigToTemplate(outputDir, registry); err != nil {
		return err
	}

	if err := plugin.ApplyTemplate(outputDir); err != nil {
		return err
	}

	return nil
}

func (r *NetworkPluginsReconciler) parseNewPlugins(req *PluginsUpdateInfo, fileList *[]string) error {
	if err := os.MkdirAll(CreateDir, os.ModePerm); err != nil {
		return err
	}

	customRegistry := req.currentSpec.Registry
	if customRegistry != "" {
		r.Log.Info("Custom registry is set ", "privateRegistryBase", req.currentSpec.Registry)
	} else {
		r.Log.Info("No custom registry is set, using defaults from image")
	}
	r.Log.Info("new plugins: ", "plugins", req.currentSpec.Plugins)

	if plugins := req.currentSpec.Plugins; plugins != nil {
		if plugins.Multus != nil {
			multusConfig := (*MultusT)(plugins.Multus)
			err := r.createPlugin(multusConfig, customRegistry)
			if err != nil {
				fmt.Printf("error: %s\n", err)
				return err
			}
			*fileList = append(*fileList, "multus.yaml")
		}

		if plugins.Sriov != nil {
			sriovConfig := (*SriovT)(plugins.Sriov)
			err := r.createPlugin(sriovConfig, customRegistry)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "sriov-cni.yaml")
			*fileList = append(*fileList, "sriov-deviceplugin.yaml")
		}

		if plugins.Whereabouts != nil {
			whConfig := (*WhereaboutsT)(plugins.Whereabouts)
			err := r.createPlugin(whConfig, customRegistry)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "whereabouts.yaml")
		}

		if plugins.OVS != nil {
			ovsConfig := (*OvsT)(plugins.OVS)
			err := r.createPlugin(ovsConfig, customRegistry)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "ovs-daemons.yaml")
			*fileList = append(*fileList, "ovs-cni.yaml")
		}

		if plugins.HostPlumber != nil {
			hostPlumberConfig := (*HostPlumberT)(plugins.HostPlumber)
			err := r.createPlugin(hostPlumberConfig, customRegistry)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "hostplumber.yaml")
		}

		if plugins.DhcpController != nil {
			dhcpControllerConfig := (*DhcpControllerT)(plugins.DhcpController)
			err := r.createPlugin(dhcpControllerConfig, customRegistry)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "dhcpcontroller.yaml")
		}

		if plugins.NodeFeatureDiscovery != nil {
			nfdConfig := (*NodeFeatureDiscoveryT)(plugins.NodeFeatureDiscovery)
			err := r.createPlugin(nfdConfig, customRegistry)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "nfd.yaml")
		}
	}
	return nil
}

func (r *NetworkPluginsReconciler) parseMissingPlugins(req *PluginsUpdateInfo, fileList *[]string) error {
	// First find out which plugins are missing from new spec vs old spec
	if req.prevSpec == nil || req.prevSpec.Plugins == nil {
		// Old spec was empty, nothing to delete
		return nil
	}

	customRegistry := req.currentSpec.Registry
	if customRegistry != "" {
		r.Log.Info("Custom registry is set ", "registryPrefix", req.currentSpec.Registry)
	} else {
		r.Log.Info("No custom registry is set, using defaults from image")
	}

	old := req.prevSpec.Plugins
	os.MkdirAll(DeleteDir, os.ModePerm)

	noOldPlugins := req.currentSpec.Plugins == nil

	if (noOldPlugins == true || req.currentSpec.Plugins.Multus == nil) && old.Multus != nil {
		multusConfig := (*MultusT)(old.Multus)
		err := r.deletePlugin(multusConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "multus.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.Whereabouts == nil) && old.Whereabouts != nil {
		whereaboutsConfig := (*WhereaboutsT)(old.Whereabouts)
		err := r.deletePlugin(whereaboutsConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "whereabouts.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.Sriov == nil) && old.Sriov != nil {
		sriovConfig := (*SriovT)(old.Sriov)
		err := r.deletePlugin(sriovConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "sriov-cni.yaml")
		*fileList = append(*fileList, "sriov-deviceplugin.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.OVS == nil) && old.OVS != nil {
		ovsConfig := (*OvsT)(old.OVS)
		err := r.deletePlugin(ovsConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "ovs-cni.yaml")
		*fileList = append(*fileList, "ovs-daemons.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.HostPlumber == nil) && old.HostPlumber != nil {
		hostPlumberConfig := (*HostPlumberT)(old.HostPlumber)
		err := r.deletePlugin(hostPlumberConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "hostplumber.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.DhcpController == nil) && old.DhcpController != nil {
		dhcpControllerConfig := (*DhcpControllerT)(old.DhcpController)
		err := r.deletePlugin(dhcpControllerConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "dhcpcontroller.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.NodeFeatureDiscovery == nil) && old.NodeFeatureDiscovery != nil {
		nfdConfig := (*NodeFeatureDiscoveryT)(old.NodeFeatureDiscovery)
		err := r.deletePlugin(nfdConfig, customRegistry)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "nfd.yaml")
	}

	return nil
}

func (r *NetworkPluginsReconciler) createPlugins(fileList []string) error {
	for _, file := range fileList {
		var fullpath string
		fullpath = filepath.Join(CreateDir, file)
		yamlfile, err := ioutil.ReadFile(fullpath)

		if err != nil {
			r.Log.Info("Failed to read file:", "pluginFile", fullpath)
			return err
		}

		resource_list := []*unstructured.Unstructured{}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlfile), 4096)
		for {
			resource := unstructured.Unstructured{}
			err := decoder.Decode(&resource)
			if err == nil {
				resource_list = append(resource_list, &resource)
			} else if err == io.EOF {
				break
			} else {
				r.Log.Error(err, "Error decoding to unstructured")
				return err
			}
		}

		for _, obj := range resource_list {
			r.Log.Info("Creating unstructured obj", "obj", obj)
			err := apply.ApplyObject(context.Background(), r.Client, obj)
			if err != nil {
				r.Log.Error(err, "Error applying unstructured object")
				return err
			}
		}
	}
	return nil
}

func (r *NetworkPluginsReconciler) deleteMissingPlugins(fileList []string) error {
	for _, file := range fileList {
		var fullpath string
		fullpath = filepath.Join(DeleteDir, file)
		yamlfile, err := ioutil.ReadFile(fullpath)

		if err != nil {
			r.Log.Info("Failed to read plugin file", "pluginFile", fullpath)
			return err
		}

		resource_list := []*unstructured.Unstructured{}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlfile), 4096)
		for {
			resource := unstructured.Unstructured{}
			err := decoder.Decode(&resource)
			if err == nil {
				resource_list = append(resource_list, &resource)
			} else if err == io.EOF {
				break
			} else {
				r.Log.Error(err, "Error decoding unstructured")
				return err
			}
		}

		for _, obj := range resource_list {
			r.Log.Info("Deleting unstructured obj", "obj", obj)
			err := apply.DeleteObject(context.Background(), r.Client, obj)
			if err != nil {
				r.Log.Error(err, "Error deleting unstructured object")
				return err
			}
		}
	}
	return nil
}

func (r *NetworkPluginsReconciler) TeardownPlugins(req *PluginsUpdateInfo) error {
	var activePlugins []string
	var deleteInfo *PluginsUpdateInfo = new(PluginsUpdateInfo)
	deleteInfo.NamespacedName = req.NamespacedName
	deleteInfo.prevSpec = req.prevSpec
	deleteInfo.currentSpec = &plumberv1.NetworkPluginsSpec{}

	if err := r.parseMissingPlugins(deleteInfo, &activePlugins); err != nil {
		r.Log.Error(err, "Could not parse plugins to delete")
		return err
	}

	if err := r.deleteMissingPlugins(activePlugins); err != nil {
		r.Log.Error(err, "Could not delete all active plugins")
		return err
	}

	cm := &corev1.ConfigMap{}
	cm.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}
	cm.ObjectMeta = metav1.ObjectMeta{Name: NetworkPluginsConfigMap, Namespace: deleteInfo.NamespacedName.Namespace}
	if err := r.Delete(context.TODO(), cm); err != nil {
		r.Log.Error(err, "Could not delete NetworkPlugins ConfigMap")
		return err
	}
	return nil
}

func (r *NetworkPluginsReconciler) saveSpecConfig(ctx context.Context, plugins *PluginsUpdateInfo) error {
	jsonSpec, err := json.Marshal(plugins.currentSpec)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	cm.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}
	cm.ObjectMeta = metav1.ObjectMeta{Name: NetworkPluginsConfigMap, Namespace: plugins.NamespacedName.Namespace}
	cm.Data = map[string]string{"currentSpec": string(jsonSpec)}

	oldcm := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: NetworkPluginsConfigMap, Namespace: plugins.NamespacedName.Namespace}, oldcm)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Did not find find previous spec, Creating...")
		if err := r.Create(ctx, cm); err != nil {
			r.Log.Error(err, "Failed to save new Spec to ConfigMap")
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Failed to get pre-existing ConfigMap")
	}

	if reflect.DeepEqual(cm, oldcm) == false {
		if err := r.Update(ctx, cm); err != nil {
			r.Log.Error(err, "Failed to update spec to ConfigMap", "configMap", cm)
			return err
		}
	} else {
		// TODO: Spec's are equal... do nothing? Or still keep track of some versioning/index???
		r.Log.Info("New Plugins spec is unchanged, not updating ConfigMap")
	}
	return nil
}

func (r *NetworkPluginsReconciler) getCurrentConfig(ctx context.Context, req ctrl.Request) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	nsm := types.NamespacedName{Name: NetworkPluginsConfigMap, Namespace: req.NamespacedName.Namespace}

	if err := r.Get(ctx, nsm, cm); err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Previous spec not found, fresh Operator deployment")
			return nil, nil
		}
		return nil, err
	}
	return cm, nil
}

func convertConfigMapToSpec(cm *corev1.ConfigMap) (*plumberv1.NetworkPluginsSpec, error) {
	spec := &plumberv1.NetworkPluginsSpec{}
	cmData := []byte(cm.Data["currentSpec"])

	if err := json.Unmarshal(cmData, spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func renderTemplateToFile(config map[string]interface{}, t *template.Template, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	err = t.Execute(f, config)
	if err != nil {
		fmt.Printf("template.Execute failed for file: %s err: %s\n", filename, err)
		f.Close()
		return err
	}
	f.Close()
	return nil
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkPluginsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.NetworkPlugins{}).
		Complete(r)
}

func GetHugepageSize() string {
	// Get the hugepage size from meminfo and convert it into bibytes from KB
	var hugepagesize int64
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		fmt.Printf("unable to open /proc/meminfo:%s\n", err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if bytes.HasPrefix(s.Bytes(), []byte(`Hugepagesize:`)) {
			_, err = fmt.Sscanf(s.Text(), "Hugepagesize:%d", &hugepagesize)
			if err != nil {
				fmt.Printf("unable to read Hugepagesize from /proc/info: %s\n", err)
			}
			break
		}
	}
	if err = s.Err(); err != nil {
		fmt.Printf("scanner error: %s\n", err)
	}
	// Converting size in KB to bibytes annotation. For example 1.0 GiB -> 1Gi
	r := humanize.BigIBytes(big.NewInt(hugepagesize * 1024))
	r = strings.Replace(r, ".0 ", "", 1)
	r = strings.Replace(r, "B", "", 1)
	fmt.Printf("Hugepages: %+v", r)
	return r
}

/*


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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"text/template"

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

	plumberv1 "github.com/platform9/luigi/api/v1"
	"github.com/platform9/luigi/pkg/apply"
)

const (
	DefaultNamespace        = "default"
	MultusImage             = "nfvpe/multus:v3.6"
	WhereaboutsImage        = "platform9/whereabouts:latest"
	SriovCniImage           = "nfvpe/sriov-cni"
	SriovDpImage            = "nfvpe/sriov-device-plugin:v3.2"
	HostplumberImage        = "platform9/luigi-plumber:latest"
	NfdImage                = "k8s.gcr.io/nfd/node-feature-discovery:v0.6.0"
	TemplateDir             = "/etc/plugin_templates/"
	CreateDir               = TemplateDir + "create/"
	DeleteDir               = TemplateDir + "delete/"
	NetworkPluginsConfigMap = "pf9-networkplugins-config"
)

// NetworkPluginsReconciler reconciles a NetworkPlugins object
type NetworkPluginsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

type PluginsUpdateContainer struct {
	Log            logr.Logger
	NamespacedName types.NamespacedName
	currentSpec    *plumberv1.NetworkPluginsSpec
	prevSpec       *plumberv1.NetworkPluginsSpec
}

type MultusT plumberv1.Multus
type WhereaboutsT plumberv1.Whereabouts
type SriovT plumberv1.Sriov
type HostPlumberT plumberv1.HostPlumber
type NodeFeatureDiscoveryT plumberv1.NodeFeatureDiscovery

type ApplyPlugin interface {
	WriteConfigToTemplate(string) error
	ApplyTemplate(string) error
}

// +kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkplugins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=networkplugins/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *NetworkPluginsReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("networkplugins", req.NamespacedName)

	var networkPluginsReq = plumberv1.NetworkPlugins{}
	if err := r.Get(ctx, req.NamespacedName, &networkPluginsReq); err != nil {
		log.Error(err, "unable to fetch NetworkPlugins")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var reqContainer *PluginsUpdateContainer = new(PluginsUpdateContainer)
	reqContainer.Log = log
	reqContainer.NamespacedName = req.NamespacedName
	reqContainer.prevSpec = nil
	reqContainer.currentSpec = &networkPluginsReq.Spec
	cm, err := r.getCurrentConfig(ctx, req)
	if err != nil {
		// TODO - Error fetching previous config (NOT a not found error) - ignore?
		log.Info("Error fetching previous ConfigMap, ignoring...")
	}
	if cm != nil {
		reqContainer.prevSpec, err = convertConfigMapToSpec(cm)
		if err != nil {
			log.Error(err, "Error converting previous ConfigMap to Spec")
			return ctrl.Result{}, err
		}
	}

	var fileList []string
	err = r.parseNewPlugins(reqContainer, &fileList)
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
	err = r.parseMissingPlugins(reqContainer, &fileListMissing)
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
	err = r.saveSpecConfig(ctx, reqContainer)
	if err != nil {
		log.Error(err, "Failed to save spec in new ConfigMap")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (hostPlumberConfig *HostPlumberT) WriteConfigToTemplate(outputDir string) error {
	config := make(map[string]interface{})
	if hostPlumberConfig.Namespace != "" {
		config["Namespace"] = hostPlumberConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if hostPlumberConfig.HostPlumberImage != "" {
		config["HostPlumberImage"] = hostPlumberConfig.HostPlumberImage
	} else {
		config["HostPlumberImage"] = HostplumberImage
	}

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

func (nfdConfig *NodeFeatureDiscoveryT) WriteConfigToTemplate(outputDir string) error {
	config := make(map[string]interface{})

	if nfdConfig.NfdImage != "" {
		config["NfdImage"] = nfdConfig.NfdImage
	} else {
		config["NfdImage"] = NfdImage
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

func (multusConfig *MultusT) WriteConfigToTemplate(outputDir string) error {
	config := make(map[string]interface{})
	if multusConfig.Namespace != "" {
		config["Namespace"] = multusConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if multusConfig.MultusImage != "" {
		config["MultusImage"] = multusConfig.MultusImage
	} else {
		config["MultusImage"] = MultusImage
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

func (whereaboutsConfig *WhereaboutsT) WriteConfigToTemplate(outputDir string) error {
	config := make(map[string]interface{})
	if whereaboutsConfig.Namespace != "" {
		config["Namespace"] = whereaboutsConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if whereaboutsConfig.WhereaboutsImage != "" {
		config["WhereaboutsImage"] = whereaboutsConfig.WhereaboutsImage
	} else {
		config["WhereaboutsImage"] = WhereaboutsImage
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

func (sriovConfig *SriovT) WriteConfigToTemplate(outputDir string) error {
	config := make(map[string]interface{})

	if sriovConfig.Namespace != "" {
		config["Namespace"] = sriovConfig.Namespace
	} else {
		config["Namespace"] = DefaultNamespace
	}

	if sriovConfig.SriovCniImage != "" {
		config["SriovCniImage"] = sriovConfig.SriovCniImage
	} else {
		config["SriovCniImage"] = SriovCniImage
	}

	if sriovConfig.SriovDpImage != "" {
		config["SriovDpImage"] = sriovConfig.SriovDpImage
	} else {
		config["SriovDpImage"] = SriovDpImage
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

func (r *NetworkPluginsReconciler) createPlugin(plugin ApplyPlugin) error {
	outputDir := CreateDir

	if err := plugin.WriteConfigToTemplate(outputDir); err != nil {
		fmt.Printf("WriteConfigToTemplate returned error: %s\n", err)
		return err
	}

	if err := plugin.ApplyTemplate(outputDir); err != nil {
		return err
	}

	return nil
}

func (r *NetworkPluginsReconciler) deletePlugin(plugin ApplyPlugin) error {
	outputDir := DeleteDir

	if err := plugin.WriteConfigToTemplate(outputDir); err != nil {
		return err
	}

	if err := plugin.ApplyTemplate(outputDir); err != nil {
		return err
	}

	return nil
}

func (r *NetworkPluginsReconciler) parseNewPlugins(req *PluginsUpdateContainer, fileList *[]string) error {
	if err := os.MkdirAll(CreateDir, os.ModePerm); err != nil {
		return err
	}

	r.Log.Info("new plugins: ", "plugins", req.currentSpec.Plugins)

	if plugins := req.currentSpec.Plugins; plugins != nil {
		if plugins.Multus != nil {
			multusConfig := (*MultusT)(plugins.Multus)
			err := r.createPlugin(multusConfig)
			if err != nil {
				fmt.Printf("error: %s\n", err)
				return err
			}
			*fileList = append(*fileList, "multus.yaml")
		}

		if plugins.Sriov != nil {
			sriovConfig := (*SriovT)(plugins.Sriov)
			err := r.createPlugin(sriovConfig)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "sriov-cni.yaml")
			*fileList = append(*fileList, "sriov-deviceplugin.yaml")
		}

		if plugins.Whereabouts != nil {
			whConfig := (*WhereaboutsT)(plugins.Whereabouts)
			err := r.createPlugin(whConfig)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "whereabouts.yaml")
		}

		if plugins.HostPlumber != nil {
			hostPlumberConfig := (*HostPlumberT)(plugins.HostPlumber)
			err := r.createPlugin(hostPlumberConfig)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "hostplumber.yaml")
		}

		if plugins.NodeFeatureDiscovery != nil {
			nfdConfig := (*NodeFeatureDiscoveryT)(plugins.NodeFeatureDiscovery)
			err := r.createPlugin(nfdConfig)
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "nfd.yaml")
		}
	}
	return nil
}

func (r *NetworkPluginsReconciler) parseMissingPlugins(req *PluginsUpdateContainer, fileList *[]string) error {
	// First find out which plugins are missing from new spec vs old spec
	if req.prevSpec == nil || req.prevSpec.Plugins == nil {
		// Old spec was empty, nothing to delete
		return nil
	}

	old := req.prevSpec.Plugins
	os.MkdirAll(DeleteDir, os.ModePerm)

	noOldPlugins := req.currentSpec.Plugins == nil

	if (noOldPlugins == true || req.currentSpec.Plugins.Multus == nil) && old.Multus != nil {
		multusConfig := (*MultusT)(old.Multus)
		err := r.deletePlugin(multusConfig)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "multus.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.Whereabouts == nil) && old.Whereabouts != nil {
		whereaboutsConfig := (*WhereaboutsT)(old.Whereabouts)
		err := r.deletePlugin(whereaboutsConfig)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "whereabouts.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.Sriov == nil) && old.Sriov != nil {
		sriovConfig := (*SriovT)(old.Sriov)
		err := r.deletePlugin(sriovConfig)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "sriov-cni.yaml")
		*fileList = append(*fileList, "sriov-deviceplugin.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.HostPlumber == nil) && old.HostPlumber != nil {
		hostPlumberConfig := (*HostPlumberT)(old.HostPlumber)
		err := r.deletePlugin(hostPlumberConfig)
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "hostplumber.yaml")
	}

	if (noOldPlugins == true || req.currentSpec.Plugins.NodeFeatureDiscovery == nil) && old.NodeFeatureDiscovery != nil {
		nfdConfig := (*NodeFeatureDiscoveryT)(old.NodeFeatureDiscovery)
		err := r.deletePlugin(nfdConfig)
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

func (r *NetworkPluginsReconciler) saveSpecConfig(ctx context.Context, plugins *PluginsUpdateContainer) error {
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

func (r *NetworkPluginsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.NetworkPlugins{}).
		Complete(r)
}

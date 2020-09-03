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
	"reflect"
	"text/template"
	//"os"
	//"path/filepath"
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
	"github.com/platform9/luigi/apply"
)

const (
	DEFAULT_NAMESPACE        = "kube-system"
	MULTUS_IMAGE             = "nfvpe/multus:v3.6"
	WHEREABOUTS_IMAGE        = "dougbtv/whereabouts:latest"
	SRIOV_CNI_IMAGE          = "nfvpe/sriov-cni"
	SRIOV_DP_IMAGE           = "nfvpe/sriov-device-plugin:v3.2"
	CONFIG_MAP_NAME          = "pf9-networkplugins-config"
	TEMPLATE_DIR             = "/etc/plugin_templates/"
	APPLY_DIR                = TEMPLATE_DIR + "apply/"
	DELETE_DIR               = TEMPLATE_DIR + "delete/"
	NETWORKPLUGINS_CONFIGMAP = "pf9-networkplugins-configmap"
)

// NetworkPluginsReconciler reconciles a NetworkPlugins object
type NetworkPluginsReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	currentSpec *plumberv1.NetworkPluginsSpec
	prevSpec    *plumberv1.NetworkPluginsSpec
}

type MultusT plumberv1.Multus
type WhereaboutsT plumberv1.Whereabouts
type SriovT plumberv1.Sriov

type ApplyPlugin interface {
	ParseTemplate(string) error
	ApplyTemplate(string) error
}

func createFile(config map[string]interface{}, t *template.Template, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	err = t.Execute(f, config)
	if err != nil {
		fmt.Printf("template.Execute failed: %s\n", err)
		f.Close()
		return err
	}
	f.Close()
	return nil
}

func (multusConfig *MultusT) ParseTemplate(outputDir string) error {
	config := make(map[string]interface{})
	if multusConfig.Namespace != "" {
		config["Namespace"] = multusConfig.Namespace
	} else {
		config["Namespace"] = DEFAULT_NAMESPACE
	}

	if multusConfig.MultusImage != "" {
		config["MultusImage"] = multusConfig.MultusImage
	} else {
		config["MultusImage"] = MULTUS_IMAGE
	}

	t, err := template.ParseFiles(TEMPLATE_DIR + "multus/multus.yaml")
	if err != nil {
		fmt.Printf("ERROR PARSEFILE: %v\n", err)
		return err
	}

	if err := createFile(config, t, outputDir+"multus.yaml"); err != nil {
		return err
	}
	return nil
}

func (multusConfig *MultusT) ApplyTemplate(outputDir string) error {
	fmt.Println("Applying Multus")
	return nil
}

func (whereaboutsConfig *WhereaboutsT) ParseTemplate(outputDir string) error {
	config := make(map[string]interface{})
	fmt.Println("Rendering Whereabouts...")
	if whereaboutsConfig.Namespace != "" {
		config["Namespace"] = whereaboutsConfig.Namespace
	} else {
		config["Namespace"] = DEFAULT_NAMESPACE
	}

	if whereaboutsConfig.WhereaboutsImage != "" {
		config["WhereaboutsImage"] = whereaboutsConfig.WhereaboutsImage
	} else {
		config["WhereaboutsImage"] = WHEREABOUTS_IMAGE
	}

	t, err := template.ParseFiles(TEMPLATE_DIR + "whereabouts/whereabouts.yaml")
	if err != nil {
		fmt.Printf("ERROR PARSEFILE: %v\n", err)
		return err
	}

	if err := createFile(config, t, outputDir+"whereabouts.yaml"); err != nil {
		return err
	}
	return nil
}

func (whereaboutsConfig *WhereaboutsT) ApplyTemplate(outputDir string) error {
	fmt.Println("Applying Whereabouts")
	return nil
}

func (sriovConfig *SriovT) ParseTemplate(outputDir string) error {
	config := make(map[string]interface{})
	fmt.Println("Rendering Sriov...")

	if sriovConfig.Namespace != "" {
		config["Namespace"] = sriovConfig.Namespace
	} else {
		config["Namespace"] = DEFAULT_NAMESPACE
	}

	if sriovConfig.SriovCniImage != "" {
		config["SriovCniImage"] = sriovConfig.SriovCniImage
	} else {
		config["SriovCniImage"] = SRIOV_CNI_IMAGE
	}

	if sriovConfig.SriovDpImage != "" {
		config["SriovDpImage"] = sriovConfig.SriovDpImage
	} else {
		config["SriovDpImage"] = SRIOV_DP_IMAGE
	}

	// Apply the SRIOV CNI
	t, err := template.ParseFiles(TEMPLATE_DIR + "sriov/sriov-cni.yaml")
	if err != nil {
		fmt.Printf("ERROR PARSEFILE: %v\n", err)
		return err
	}

	if err := createFile(config, t, outputDir+"sriov-cni.yaml"); err != nil {
		return err
	}

	// Apply the SRIOV Device Plugin
	t, err = template.ParseFiles(TEMPLATE_DIR + "sriov/sriov-deviceplugin.yaml")
	if err != nil {
		fmt.Printf("ERROR PARSEFILE: %v\n", err)
		return err
	}

	if err := createFile(config, t, outputDir+"sriov-deviceplugin.yaml"); err != nil {
		return err
	}
	return nil
}

func (sriovConfig *SriovT) ApplyTemplate(outputDir string) error {
	fmt.Println("Applying SRIOV")
	return nil
}

func (r *NetworkPluginsReconciler) parsePlugin(plugin ApplyPlugin, mode string) error {
	var outputDir string
	switch mode {
	case "create":
		outputDir = APPLY_DIR
	case "delete":
		outputDir = DELETE_DIR
	default:
		fmt.Printf("Invalid option to parsePlugin: %s\n", mode)
		return errors.NewBadRequest("Invalid option to parsePlugin")
	}

	if err := plugin.ParseTemplate(outputDir); err != nil {
		return err
	}

	if err := plugin.ApplyTemplate(outputDir); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=plumber.k8snetworking.pf9.io,resources=networkplugins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=plumber.k8snetworking.pf9.io,resources=networkplugins/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *NetworkPluginsReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("networkplugins", req.NamespacedName)

	var networkPluginsReq = plumberv1.NetworkPlugins{}
	if err := r.Get(ctx, req.NamespacedName, &networkPluginsReq); err != nil {
		log.Error(err, "unable to fetch NetworkPlugins")
		fmt.Println("Unable to fetch new NetworkPlugins CRD!!!")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	r.currentSpec = &networkPluginsReq.Spec
	cm, err := r.getCurrentConfig(ctx, req)
	if err != nil {
		// TODO - Error fetching previous config (NOT a not found error) - ignore?
		fmt.Printf("Error fetching previous ConfigMap, ignoring...\n")
	}
	if cm != nil {
		r.prevSpec, err = convertConfigMapToSpec(cm)
		if err != nil {
			fmt.Printf("Error converting previous ConfigMap to Spec\n")
			return ctrl.Result{}, err
		}
	}

	var fileList []string
	err = r.parseNewTemplates(&fileList)
	if err != nil {
		fmt.Printf("Error applying templates! %v\n", err)
		return ctrl.Result{}, err
	}
	fmt.Printf("Got fileList = %s\n", fileList)
	err = r.applyDeleteTemplates(&networkPluginsReq, fileList, "create")
	if err != nil {
		return ctrl.Result{}, err
	}

	var fileListDelete []string
	err = r.parseOldTemplates(&fileListDelete)
	if err != nil {
		fmt.Printf("Error applying templates! %v\n", err)
		return ctrl.Result{}, err
	}
	fmt.Printf("Got fileList = %s\n", fileList)
	err = r.applyDeleteTemplates(&networkPluginsReq, fileListDelete, "delete")
	if err != nil {
		return ctrl.Result{}, err
	}

	// Everything succeeded - save Spec we just applied to ConfigMap
	err = r.saveSpecConfig(ctx, req)
	if err != nil {
		fmt.Printf("Failed to save spec in new ConfigMap: %s\n", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPluginsReconciler) parseNewTemplates(fileList *[]string) error {
	var spec *plumberv1.NetworkPluginsSpec = r.currentSpec

	os.MkdirAll(APPLY_DIR, os.ModePerm)

	if plugins := spec.CniPlugins; plugins != nil {
		if plugins.Multus != nil {
			multusConfig := (*MultusT)(plugins.Multus)
			err := r.parsePlugin(multusConfig, "create")
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "multus.yaml")
		}

		if plugins.Sriov != nil {
			sriovConfig := (*SriovT)(plugins.Sriov)
			err := r.parsePlugin(sriovConfig, "create")
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "sriov-cni.yaml")
			*fileList = append(*fileList, "sriov-deviceplugin.yaml")
		}

		if plugins.Whereabouts != nil {
			whConfig := (*WhereaboutsT)(plugins.Whereabouts)
			err := r.parsePlugin(whConfig, "create")
			if err != nil {
				return err
			}
			*fileList = append(*fileList, "whereabouts.yaml")
		}
	}
	return nil
}

func (r *NetworkPluginsReconciler) parseOldTemplates(fileList *[]string) error {
	// First find out which plugins are missing from new spec vs old spec
	if r.prevSpec == nil || r.prevSpec.CniPlugins == nil {
		// Old spec was empty, nothing to delete
		return nil
	}

	old := r.prevSpec.CniPlugins
	os.MkdirAll(DELETE_DIR, os.ModePerm)

	var noCni bool
	if r.currentSpec.CniPlugins == nil {
		noCni = true
	} else {
		noCni = false
	}

	if (noCni == true || r.currentSpec.CniPlugins.Multus == nil) && old.Multus != nil {
		multusConfig := (*MultusT)(old.Multus)
		err := r.parsePlugin(multusConfig, "delete")
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "multus.yaml")
	}

	if (noCni == true || r.currentSpec.CniPlugins.Whereabouts == nil) && old.Whereabouts != nil {
		whereaboutsConfig := (*WhereaboutsT)(old.Whereabouts)
		err := r.parsePlugin(whereaboutsConfig, "delete")
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "whereabouts.yaml")
	}

	if (noCni == true || r.currentSpec.CniPlugins.Sriov == nil) && old.Sriov != nil {
		sriovConfig := (*SriovT)(old.Sriov)
		err := r.parsePlugin(sriovConfig, "delete")
		if err != nil {
			return err
		}
		*fileList = append(*fileList, "sriov-cni.yaml")
		*fileList = append(*fileList, "sriov-deviceplugin.yaml")
	}
	return nil
}

func (r *NetworkPluginsReconciler) applyDeleteTemplates(networkPlugins *plumberv1.NetworkPlugins, fileList []string, mode string) error {
	for _, file := range fileList {
		var fullpath string

		switch mode {
		case "create":
			fullpath = APPLY_DIR + file
		case "delete":
			fullpath = DELETE_DIR + file
		default:
			fmt.Printf("Invalid mode to applyDeleteTemplate: %s\n", mode)
			return errors.NewBadRequest("Invalid option to applyDeleteTemplate")
		}

		fmt.Printf("mode = %s YAML file: %s\n", mode, fullpath)
		yamlfile, err := ioutil.ReadFile(fullpath)

		if err != nil {
			fmt.Printf("Failed to read file: %s\n", fullpath)
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
				fmt.Printf("Error decoding!!!: %s\n", err)
				return err
			}
		}

		for _, obj := range resource_list {
			if mode == "create" {
				fmt.Printf("Creating obj: %+v\n", obj)
				err := apply.ApplyObject(context.Background(), r.Client, obj)
				if err != nil {
					fmt.Printf("Error creating object: %+v\n\nError: %s\n", obj, err)
					return err
				}
			} else if mode == "delete" {
				fmt.Printf("Deleting obj: %+v\n", obj)
				err := apply.DeleteObject(context.Background(), r.Client, obj)
				if err != nil {
					fmt.Printf("Error deleting object: %+v\n\nError: %s\n", obj, err)
					return err
				}
			}
		}
	}
	return nil
}

func (r *NetworkPluginsReconciler) saveSpecConfig(ctx context.Context, req ctrl.Request) error {

	jsonSpec, err := json.Marshal(r.currentSpec)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	cm.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}
	cm.ObjectMeta = metav1.ObjectMeta{Name: NETWORKPLUGINS_CONFIGMAP, Namespace: req.NamespacedName.Namespace}
	cm.Data = map[string]string{"currentSpec": string(jsonSpec)}

	oldcm := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: NETWORKPLUGINS_CONFIGMAP, Namespace: req.NamespacedName.Namespace}, oldcm)
	if err != nil && errors.IsNotFound(err) {
		fmt.Printf("Did not find find previous spec, Creating...\n")
		if err := r.Create(ctx, cm); err != nil {
			fmt.Printf("Failed to save new Spec to ConfigMap: %s\n", err)
			return err
		}
	} else if err != nil {
		fmt.Printf("Failed to get pre-existing ConfigMap: %s\n", err)
	}

	if reflect.DeepEqual(cm, oldcm) == false {
		if err := r.Update(ctx, cm); err != nil {
			fmt.Printf("Failed to Update config: %+v\n", cm)
			return err
		}
	} else {
		// TODO: Spec's are equal... do nothing? Or still keep track of some versioning/index???
		fmt.Printf("Specs are equal... not saving\n")
	}
	return nil
}

func (r *NetworkPluginsReconciler) getCurrentConfig(ctx context.Context, req ctrl.Request) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	nsm := types.NamespacedName{Name: NETWORKPLUGINS_CONFIGMAP, Namespace: req.NamespacedName.Namespace}

	if err := r.Get(ctx, nsm, cm); err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("Previous spec not found, fresh Operator deployment\n")
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

func (r *NetworkPluginsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.NetworkPlugins{}).
		Complete(r)
}

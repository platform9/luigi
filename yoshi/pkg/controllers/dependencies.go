package controllers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO: yes, hardcoded for now. Move to luigi and addon-operator once release is stable
const TemplateDir = "/etc/plugin_templates/"
const DeploymentWaitTimeout = 60

var plugins = []string{"calico-apiserver.yaml"}

type DeployCtx struct {
	Client      client.Client
	Log         logr.Logger
	pluginPaths map[string]string
}

func NewDeployCtx(log logr.Logger, client client.Client) *DeployCtx {
	ctx := new(DeployCtx)
	for _, pluginFile := range plugins {
		split := strings.Split(pluginFile, ".")
		pluginName := split[0]
		ctx.pluginPaths = make(map[string]string)
		ctx.pluginPaths[pluginName] = pluginFile
	}

	ctx.Log = log
	ctx.Client = client
	return ctx
}

func (ctx *DeployCtx) DeployDependencies() error {
	for pluginName, pluginFile := range ctx.pluginPaths {
		fullpath := filepath.Join(TemplateDir, pluginFile)
		yamlfile, err := ioutil.ReadFile(fullpath)

		ctx.Log.Info("Deploying plugin:", "plugin", pluginName, "path", fullpath)

		if err != nil {
			ctx.Log.Info("Failed to read file:", "pluginFile", fullpath)
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
				ctx.Log.Error(err, "Error decoding to unstructured")
				return err
			}
		}

		for _, obj := range resource_list {
			ctx.Log.Info("Creating unstructured obj", "obj", obj)
			err := ApplyObject(context.Background(), ctx.Client, obj)
			if err != nil {
				ctx.Log.Error(err, "Error applying unstructured object")
				return err
			}
		}

		// TODO: For some reason this does not work because client cache is not init'd yet
		// But client cache is not init'd until manager is Started, but that never returns
		// For now assume the deployment always succeeds. Could not find a way to init client cache
		/*
			if err := ctx.WaitForDeployment(pluginName, pluginName, DeploymentWaitTimeout); err != nil {
				ctx.Log.Error(err, "deployment not ready", "name", pluginName)
				return err
			}
		*/

	}
	return nil
}

func (ctx *DeployCtx) WaitForDeployment(name, namespace string, timeoutSeconds int) error {
	ch := make(chan error)
	go ctx._waitForDeployment(ch, name, namespace)

	select {
	case err := <-ch:
		if err != nil {
			return fmt.Errorf("failed to find deployment %s: %v",
				name, err)
		}
		ctx.Log.Info("found deployment with running pods", "name", name)
		return nil

	case <-time.After(time.Second * time.Duration(timeoutSeconds)):
		return fmt.Errorf("failed to find running deployment %s", name)

	}
}

func (ctx *DeployCtx) _waitForDeployment(ch chan error, name, namespace string) {
	for {
		deployment, err := ctx.GetDeployment(name, namespace)
		if err != nil {
			ctx.Log.Error(err, "failed to find deployment", "name", name)
		} else if deployment.Status.ReadyReplicas > 0 {
			ch <- nil
			return
		}
		time.Sleep(time.Second)
	}
}

func (ctx *DeployCtx) GetDeployment(name, namespace string) (*appsv1.Deployment, error) {
	//deployment, err := ctx.clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})

	deployment := &appsv1.Deployment{}
	nsm := types.NamespacedName{Name: name, Namespace: namespace}
	err := ctx.Client.Get(context.Background(), nsm, deployment)
	if err != nil {
		ctx.Log.Error(err, "failed to get deployment", "key", nsm)
		return nil, err
	}

	return deployment, err
}

// DeleteObject deletes the desired object against the apiserver,
func DeleteObject(ctx context.Context, client client.Client, obj *uns.Unstructured) error {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	if name == "" {
		return errors.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}
	gvk := obj.GroupVersionKind()
	// used for logging and errors
	objDesc := fmt.Sprintf("(%s) %s/%s", gvk.String(), namespace, name)
	log.Printf("reconciling %s", objDesc)

	if err := IsObjectSupported(obj); err != nil {
		return errors.Wrapf(err, "object %s unsupported", objDesc)
	}

	// Get existing
	existing := &uns.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)

	if err != nil && apierrors.IsNotFound(err) {
		log.Printf("does not exist, do nothing %s", objDesc)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "could not retrieve existing %s", objDesc)
	}

	if err = client.Delete(ctx, existing); err != nil {
		return errors.Wrapf(err, "could not delete object %s", objDesc)
	} else {
		log.Printf("delete was successful")
	}
	return nil
}

// ApplyObject applies the desired object against the apiserver,
// merging it with any existing objects if already present.
func ApplyObject(ctx context.Context, client client.Client, obj *uns.Unstructured) error {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	if name == "" {
		return errors.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}
	gvk := obj.GroupVersionKind()
	// used for logging and errors
	objDesc := fmt.Sprintf("(%s) %s/%s", gvk.String(), namespace, name)
	log.Printf("reconciling %s", objDesc)

	if err := IsObjectSupported(obj); err != nil {
		return errors.Wrapf(err, "object %s unsupported", objDesc)
	}

	// Get existing
	existing := &uns.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)

	if err != nil && apierrors.IsNotFound(err) {
		log.Printf("does not exist, creating %s", objDesc)
		err := client.Create(ctx, obj)
		if err != nil {
			return errors.Wrapf(err, "could not create %s", objDesc)
		}
		log.Printf("successfully created %s", objDesc)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "could not retrieve existing %s", objDesc)
	}

	// Merge the desired object with what actually exists
	if err := MergeObjectForUpdate(existing, obj); err != nil {
		return errors.Wrapf(err, "could not merge object %s with existing", objDesc)
	}
	if !equality.Semantic.DeepDerivative(obj, existing) {
		if err := client.Update(ctx, obj); err != nil {
			return errors.Wrapf(err, "could not update object %s", objDesc)
		} else {
			log.Printf("update was successful")
		}
	}

	return nil
}

// MergeMetadataForUpdate merges the read-only fields of metadata.
// This is to be able to do a a meaningful comparison in apply,
// since objects created on runtime do not have these fields populated.
func MergeMetadataForUpdate(current, updated *uns.Unstructured) error {
	updated.SetCreationTimestamp(current.GetCreationTimestamp())
	updated.SetSelfLink(current.GetSelfLink())
	updated.SetGeneration(current.GetGeneration())
	updated.SetUID(current.GetUID())
	updated.SetResourceVersion(current.GetResourceVersion())

	mergeAnnotations(current, updated)
	mergeLabels(current, updated)

	return nil
}

// MergeObjectForUpdate prepares a "desired" object to be updated.
// Some objects, such as Deployments and Services require
// some semantic-aware updates
func MergeObjectForUpdate(current, updated *uns.Unstructured) error {
	if err := MergeDeploymentForUpdate(current, updated); err != nil {
		return err
	}

	if err := MergeServiceForUpdate(current, updated); err != nil {
		return err
	}

	if err := MergeServiceAccountForUpdate(current, updated); err != nil {
		return err
	}

	// For all object types, merge metadata.
	// Run this last, in case any of the more specific merge logic has
	// changed "updated"
	MergeMetadataForUpdate(current, updated)

	return nil
}

const (
	deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"
)

// MergeDeploymentForUpdate updates Deployment objects.
// We merge annotations, keeping ours except the Deployment Revision annotation.
func MergeDeploymentForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()
	if gvk.Group == "apps" && gvk.Kind == "Deployment" {

		// Copy over the revision annotation from current up to updated
		// otherwise, updated would win, and this annotation is "special" and
		// needs to be preserved
		curAnnotations := current.GetAnnotations()
		updatedAnnotations := updated.GetAnnotations()
		if updatedAnnotations == nil {
			updatedAnnotations = map[string]string{}
		}

		anno, ok := curAnnotations[deploymentRevisionAnnotation]
		if ok {
			updatedAnnotations[deploymentRevisionAnnotation] = anno
		}

		updated.SetAnnotations(updatedAnnotations)
	}

	return nil
}

// MergeServiceForUpdate ensures the clusterip is never written to
func MergeServiceForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "Service" {
		clusterIP, found, err := uns.NestedString(current.Object, "spec", "clusterIP")
		if err != nil {
			return err
		}

		if found {
			return uns.SetNestedField(updated.Object, clusterIP, "spec", "clusterIP")
		}
	}

	return nil
}

// MergeServiceAccountForUpdate copies secrets from current to updated.
// This is intended to preserve the auto-generated token.
// Right now, we just copy current to updated and don't support supplying
// any secrets ourselves.
func MergeServiceAccountForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "ServiceAccount" {
		curSecrets, ok, err := uns.NestedSlice(current.Object, "secrets")
		if err != nil {
			return err
		}

		if ok {
			uns.SetNestedField(updated.Object, curSecrets, "secrets")
		}

		curImagePullSecrets, ok, err := uns.NestedSlice(current.Object, "imagePullSecrets")
		if err != nil {
			return err
		}
		if ok {
			uns.SetNestedField(updated.Object, curImagePullSecrets, "imagePullSecrets")
		}
	}
	return nil
}

// mergeAnnotations copies over any annotations from current to updated,
// with updated winning if there's a conflict
func mergeAnnotations(current, updated *uns.Unstructured) {
	updatedAnnotations := updated.GetAnnotations()
	curAnnotations := current.GetAnnotations()

	if curAnnotations == nil {
		curAnnotations = map[string]string{}
	}

	for k, v := range updatedAnnotations {
		curAnnotations[k] = v
	}
	if len(curAnnotations) > 1 {
		updated.SetAnnotations(curAnnotations)
	}
}

// mergeLabels copies over any labels from current to updated,
// with updated winning if there's a conflict
func mergeLabels(current, updated *uns.Unstructured) {
	updatedLabels := updated.GetLabels()
	curLabels := current.GetLabels()

	if curLabels == nil {
		curLabels = map[string]string{}
	}

	for k, v := range updatedLabels {
		curLabels[k] = v
	}
	if len(curLabels) > 1 {
		updated.SetLabels(curLabels)
	}
}

// IsObjectSupported rejects objects with configurations we don't support.
// This catches ServiceAccounts with secrets, which is valid but we don't
// support reconciling them.
func IsObjectSupported(obj *uns.Unstructured) error {
	gvk := obj.GroupVersionKind()

	// We cannot create ServiceAccounts with secrets because there's currently
	// no need and the merging logic is complex.
	// If you need this, please file an issue.
	if gvk.Group == "" && gvk.Kind == "ServiceAccount" {
		secrets, ok, err := uns.NestedSlice(obj.Object, "secrets")
		if err != nil {
			return err
		}

		if ok && len(secrets) > 0 {
			return errors.Errorf("cannot create ServiceAccount with secrets")
		}
	}

	return nil
}

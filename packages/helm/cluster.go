package helm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/chartutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

const (
	fieldManager         = "helm-controller"
	labelManagedBy       = "app.kubernetes.io/managed-by"
	annotationReleaseKey = "meta.helm.sh/release-name"
	annotationReleaseNS  = "meta.helm.sh/release-namespace"
	annotationPolicy     = "helm.sh/resource-policy"
	mapperAttempts       = 30
)

var (
	namespacesGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	namespaceGVK  = schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}
)

type cluster struct {
	client dynamic.Interface
	mapper func() (meta.RESTMapper, error)
	wait   time.Duration
	loaded meta.RESTMapper
}

func (c *cluster) mapping(ctx context.Context, gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	return c.mappingWithin(ctx, gvk, mapperAttempts)
}

func (c *cluster) mappingWithin(ctx context.Context, gvk schema.GroupVersionKind, attempts int) (*meta.RESTMapping, error) {
	for attempt := 0; ; attempt++ {
		if c.loaded == nil {
			m, err := c.mapper()
			if err != nil {
				return nil, err
			}
			c.loaded = m
		}
		mapping, err := c.loaded.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err == nil || !meta.IsNoMatchError(err) || attempt >= attempts {
			return mapping, err
		}
		c.loaded = nil
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.wait):
		}
	}
}

func (c *cluster) resource(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return c.client.Resource(mapping.Resource).Namespace(namespace)
	}
	return c.client.Resource(mapping.Resource)
}

func objectKey(obj *unstructured.Unstructured) string {
	gvk := obj.GroupVersionKind()
	return gvk.Group + "/" + gvk.Kind + "/" + obj.GetNamespace() + "/" + obj.GetName()
}

func (c *cluster) applyOwned(ctx context.Context, obj *unstructured.Unstructured, releaseName, releaseNamespace string, takeOwnership bool) error {
	mapping, err := c.mapping(ctx, obj.GroupVersionKind())
	if err != nil {
		return fmt.Errorf("%s %s: %w", obj.GetKind(), obj.GetName(), err)
	}
	if obj.GetNamespace() == "" && mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		obj.SetNamespace(releaseNamespace)
	}
	resource := c.resource(mapping, obj.GetNamespace())
	if !takeOwnership {
		existing, err := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err == nil && !ownedBy(existing, releaseName, releaseNamespace) {
			return fmt.Errorf("%s %q in namespace %q exists and cannot be imported into the current release: invalid ownership metadata", obj.GetKind(), obj.GetName(), obj.GetNamespace())
		}
	}
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[labelManagedBy] = "Helm"
	obj.SetLabels(labels)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotationReleaseKey] = releaseName
	annotations[annotationReleaseNS] = releaseNamespace
	obj.SetAnnotations(annotations)
	if _, err := resource.Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return fmt.Errorf("apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return nil
}

func ownedBy(obj *unstructured.Unstructured, releaseName, releaseNamespace string) bool {
	annotations := obj.GetAnnotations()
	return obj.GetLabels()[labelManagedBy] == "Helm" &&
		annotations[annotationReleaseKey] == releaseName &&
		annotations[annotationReleaseNS] == releaseNamespace
}

func (c *cluster) createIfMissing(ctx context.Context, obj *unstructured.Unstructured) error {
	mapping, err := c.mapping(ctx, obj.GroupVersionKind())
	if err != nil {
		return err
	}
	_, err = c.resource(mapping, obj.GetNamespace()).Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (c *cluster) remove(ctx context.Context, objs []*unstructured.Unstructured, releaseNamespace string, keep map[string]bool) error {
	seen := map[string]bool{}
	for _, obj := range objs {
		mapping, err := c.mappingWithin(ctx, obj.GroupVersionKind(), 1)
		if meta.IsNoMatchError(err) {
			continue
		}
		if err != nil {
			return err
		}
		if obj.GetNamespace() == "" && mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			obj.SetNamespace(releaseNamespace)
		}
		key := objectKey(obj)
		if keep[key] || seen[key] || obj.GetAnnotations()[annotationPolicy] == "keep" {
			continue
		}
		seen[key] = true
		err = c.resource(mapping, obj.GetNamespace()).Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

func DiscoveredCapabilities(disco discovery.DiscoveryInterface) (*chartutil.Capabilities, error) {
	version, err := disco.ServerVersion()
	if err != nil {
		return nil, err
	}
	_, lists, err := disco.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, err
	}
	caps := *chartutil.DefaultCapabilities
	caps.KubeVersion = chartutil.KubeVersion{Version: version.GitVersion, Major: version.Major, Minor: version.Minor}
	var apis chartutil.VersionSet
	for _, list := range lists {
		apis = append(apis, list.GroupVersion)
		for _, resource := range list.APIResources {
			if !strings.Contains(resource.Name, "/") {
				apis = append(apis, list.GroupVersion+"/"+resource.Kind)
			}
		}
	}
	caps.APIVersions = apis
	return &caps, nil
}

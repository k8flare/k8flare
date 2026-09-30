package addons

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	GVKAnnotation  = "addon.k3s.cattle.io/gvks"
	gvkSep         = ";"
	fieldManager   = "deploy"
	addonNamespace = metav1.NamespaceSystem

	labelHash      = "objectset.rio.cattle.io/hash"
	labelID        = "objectset.rio.cattle.io/id"
	labelOwnerGVK  = "objectset.rio.cattle.io/owner-gvk"
	labelOwnerName = "objectset.rio.cattle.io/owner-name"
	labelOwnerNS   = "objectset.rio.cattle.io/owner-namespace"
)

var (
	addonGVK  = schema.GroupVersionKind{Group: "k3s.cattle.io", Version: "v1", Kind: "Addon"}
	addonsGVR = addonGVK.GroupVersion().WithResource("addons")
	crdsGVR   = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

	ErrAddonCRDPending = errors.New("addons.k3s.cattle.io is not served yet")
)

const addonCRDName = "addons.k3s.cattle.io"

type Deployer struct {
	Client dynamic.Interface
	Mapper meta.RESTMapper
}

func (d *Deployer) Deploy(ctx context.Context, files []File, disables map[string]bool) error {
	if err := d.ensureAddonCRD(ctx); err != nil {
		return err
	}
	skips := map[string]bool{}
	for _, f := range files {
		if name, ok := strings.CutSuffix(f.Name, ".skip"); ok {
			skips[name] = true
		}
	}
	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var errs []error
	for _, f := range sorted {
		if !isManifest(f.Name) {
			continue
		}
		if disables[strings.TrimSuffix(f.Name, path.Ext(f.Name))] {
			if err := d.remove(ctx, f); err != nil {
				errs = append(errs, fmt.Errorf("delete %s: %w", f.Name, err))
			}
			continue
		}
		if skips[f.Name] {
			continue
		}
		if err := d.deploy(ctx, f); err != nil {
			errs = append(errs, fmt.Errorf("deploy %s: %w", f.Name, err))
		}
	}
	return errors.Join(errs...)
}

func isManifest(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".yaml", ".yml", ".json":
		return true
	}
	return false
}

func addonName(file string) string {
	base, _, _ := strings.Cut(path.Base(file), ".")
	return base
}

func (d *Deployer) ensureAddonCRD(ctx context.Context) error {
	crds := d.Client.Resource(crdsGVR)
	if _, err := crds.Get(ctx, addonCRDName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		objs, err := Decode(addonCRD)
		if err != nil {
			return err
		}
		if _, err := crds.Apply(ctx, addonCRDName, objs[0], applyOptions()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := d.Mapper.RESTMapping(addonGVK.GroupKind(), addonGVK.Version); err != nil {
		if meta.IsNoMatchError(err) {
			return ErrAddonCRDPending
		}
		return err
	}
	return nil
}

func applyOptions() metav1.ApplyOptions {
	return metav1.ApplyOptions{FieldManager: fieldManager, Force: true}
}

func (d *Deployer) getAddon(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	addon, err := d.Client.Resource(addonsGVR).Namespace(addonNamespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return addon, err
}

func checksumOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func ownerMetadata(name string) (map[string]string, map[string]string) {
	annotations := map[string]string{
		labelID:        "",
		labelOwnerGVK:  addonGVK.String(),
		labelOwnerName: name,
		labelOwnerNS:   addonNamespace,
	}
	digest := sha1.New()
	for _, key := range []string{labelID, labelOwnerGVK, labelOwnerName, labelOwnerNS} {
		digest.Write([]byte(annotations[key]))
	}
	return map[string]string{labelHash: hex.EncodeToString(digest.Sum(nil))}, annotations
}

func (d *Deployer) deploy(ctx context.Context, f File) error {
	name := addonName(f.Name)
	addon, err := d.getAddon(ctx, name)
	if err != nil {
		return err
	}
	checksum := checksumOf(f.Content)
	if addon != nil {
		if current, _, _ := unstructured.NestedString(addon.Object, "spec", "checksum"); current == checksum {
			return nil
		}
	}
	objs, err := Decode(f.Content)
	if err != nil {
		return err
	}
	labels, annotations := ownerMetadata(name)
	keep := map[string]bool{}
	var applied []schema.GroupVersionKind
	for _, obj := range objs {
		mapping, err := d.Mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		if err != nil {
			return err
		}
		withOwner(obj, labels, annotations)
		if _, err := d.resource(mapping, obj.GetNamespace()).Apply(ctx, obj.GetName(), obj, applyOptions()); err != nil {
			return fmt.Errorf("apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
		keep[objectKey(obj)] = true
		applied = appendGVK(applied, obj.GroupVersionKind())
	}
	if err := d.prune(ctx, labels, keep, append(previousGVKs(addon), applied...)); err != nil {
		return err
	}
	return d.saveAddon(ctx, addon, name, f.Name, checksum, applied)
}

func (d *Deployer) remove(ctx context.Context, f File) error {
	name := addonName(f.Name)
	addon, err := d.getAddon(ctx, name)
	if err != nil || addon == nil {
		return err
	}
	gvks := previousGVKs(addon)
	if objs, err := Decode(f.Content); err == nil {
		for _, obj := range objs {
			gvks = appendGVK(gvks, obj.GroupVersionKind())
		}
	}
	labels, _ := ownerMetadata(name)
	if err := d.prune(ctx, labels, map[string]bool{}, gvks); err != nil {
		return err
	}
	err = d.Client.Resource(addonsGVR).Namespace(addonNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func withOwner(obj *unstructured.Unstructured, labels, annotations map[string]string) {
	l := obj.GetLabels()
	if l == nil {
		l = map[string]string{}
	}
	for k, v := range labels {
		l[k] = v
	}
	obj.SetLabels(l)
	a := obj.GetAnnotations()
	if a == nil {
		a = map[string]string{}
	}
	for k, v := range annotations {
		a[k] = v
	}
	obj.SetAnnotations(a)
}

func (d *Deployer) resource(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return d.Client.Resource(mapping.Resource).Namespace(namespace)
	}
	return d.Client.Resource(mapping.Resource)
}

func objectKey(obj *unstructured.Unstructured) string {
	return obj.GroupVersionKind().String() + "/" + obj.GetNamespace() + "/" + obj.GetName()
}

func appendGVK(list []schema.GroupVersionKind, gvk schema.GroupVersionKind) []schema.GroupVersionKind {
	for _, have := range list {
		if have == gvk {
			return list
		}
	}
	return append(list, gvk)
}

func previousGVKs(addon *unstructured.Unstructured) []schema.GroupVersionKind {
	if addon == nil {
		return nil
	}
	var gvks []schema.GroupVersionKind
	for _, s := range strings.Split(addon.GetAnnotations()[GVKAnnotation], gvkSep) {
		if gvk, ok := parseGVK(s); ok {
			gvks = appendGVK(gvks, gvk)
		}
	}
	return gvks
}

func parseGVK(s string) (schema.GroupVersionKind, bool) {
	gv, kind, ok := strings.Cut(s, ", Kind=")
	if !ok {
		return schema.GroupVersionKind{}, false
	}
	parsed, err := schema.ParseGroupVersion(gv)
	if err != nil {
		return schema.GroupVersionKind{}, false
	}
	return parsed.WithKind(kind), true
}

func (d *Deployer) prune(ctx context.Context, labels map[string]string, keep map[string]bool, gvks []schema.GroupVersionKind) error {
	seen := map[schema.GroupVersionKind]bool{}
	for _, gvk := range gvks {
		if seen[gvk] {
			continue
		}
		seen[gvk] = true
		mapping, err := d.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if meta.IsNoMatchError(err) {
			continue
		}
		if err != nil {
			return err
		}
		list, err := d.Client.Resource(mapping.Resource).List(ctx, metav1.ListOptions{LabelSelector: labelHash + "=" + labels[labelHash]})
		if err != nil {
			return err
		}
		for i := range list.Items {
			obj := &list.Items[i]
			obj.SetGroupVersionKind(gvk)
			if keep[objectKey(obj)] {
				continue
			}
			err := d.resource(mapping, obj.GetNamespace()).Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (d *Deployer) saveAddon(ctx context.Context, addon *unstructured.Unstructured, name, source, checksum string, applied []schema.GroupVersionKind) error {
	strs := make([]string, len(applied))
	for i, gvk := range applied {
		strs[i] = gvk.String()
	}
	client := d.Client.Resource(addonsGVR).Namespace(addonNamespace)
	create := addon == nil
	if create {
		addon = &unstructured.Unstructured{Object: map[string]any{}}
		addon.SetGroupVersionKind(addonGVK)
		addon.SetName(name)
		addon.SetNamespace(addonNamespace)
	}
	annotations := addon.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[GVKAnnotation] = strings.Join(strs, gvkSep)
	addon.SetAnnotations(annotations)
	if err := unstructured.SetNestedField(addon.Object, source, "spec", "source"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(addon.Object, checksum, "spec", "checksum"); err != nil {
		return err
	}
	var err error
	if create {
		_, err = client.Create(ctx, addon, metav1.CreateOptions{})
	} else {
		_, err = client.Update(ctx, addon, metav1.UpdateOptions{})
	}
	return err
}

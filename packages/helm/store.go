package helm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"helm.sh/helm/v3/pkg/release"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	secretType   = "helm.sh/release.v1"
	historyMax   = 10
	labelOwner   = "owner"
	labelName    = "name"
	labelStatus  = "status"
	labelVersion = "version"
)

var secretsGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

type record struct {
	release *release.Release
	secret  *unstructured.Unstructured
}

func (r *record) configHash() string {
	return r.secret.GetLabels()[KeyConfigHash]
}

type store struct {
	client dynamic.Interface
	now    func() time.Time
}

func secretName(name string, revision int) string {
	return fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, revision)
}

func (s store) history(ctx context.Context, namespace, name string) ([]*record, error) {
	list, err := s.client.Resource(secretsGVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelOwner + "=helm," + labelName + "=" + name})
	if err != nil {
		return nil, err
	}
	records := make([]*record, 0, len(list.Items))
	for i := range list.Items {
		secret := &list.Items[i]
		encoded, _, _ := unstructured.NestedString(secret.Object, "data", "release")
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("release secret %s: %w", secret.GetName(), err)
		}
		rel, err := decodeRelease(string(raw))
		if err != nil {
			return nil, fmt.Errorf("release secret %s: %w", secret.GetName(), err)
		}
		records = append(records, &record{release: rel, secret: secret})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].release.Version < records[j].release.Version })
	return records, nil
}

func (s store) create(ctx context.Context, rel *release.Release) error {
	obj, err := s.secretObject(rel, map[string]string{"createdAt": strconv.FormatInt(s.now().Unix(), 10)})
	if err != nil {
		return err
	}
	_, err = s.client.Resource(secretsGVR).Namespace(rel.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

func (s store) update(ctx context.Context, rec *record) error {
	obj, err := s.secretObject(rec.release, map[string]string{"modifiedAt": strconv.FormatInt(s.now().Unix(), 10)})
	if err != nil {
		return err
	}
	labels := obj.GetLabels()
	for k, v := range rec.secret.GetLabels() {
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}
	obj.SetLabels(labels)
	obj.SetResourceVersion(rec.secret.GetResourceVersion())
	_, err = s.client.Resource(secretsGVR).Namespace(rec.release.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func (s store) supersede(ctx context.Context, rec *record) error {
	rec.release.Info.Status = release.StatusSuperseded
	return s.update(ctx, rec)
}

func (s store) delete(ctx context.Context, rec *record) error {
	err := s.client.Resource(secretsGVR).Namespace(rec.release.Namespace).Delete(ctx, rec.secret.GetName(), metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (s store) trim(ctx context.Context, records []*record) error {
	for len(records) > historyMax {
		var victim int = -1
		for i, rec := range records {
			if rec.release.Info.Status != release.StatusDeployed {
				victim = i
				break
			}
		}
		if victim < 0 {
			return nil
		}
		if err := s.delete(ctx, records[victim]); err != nil {
			return err
		}
		records = append(records[:victim], records[victim+1:]...)
	}
	return nil
}

func (s store) secretObject(rel *release.Release, extra map[string]string) (*unstructured.Unstructured, error) {
	encoded, err := encodeRelease(rel)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{}
	for k, v := range rel.Labels {
		labels[k] = v
	}
	for k, v := range extra {
		labels[k] = v
	}
	labels[labelName] = rel.Name
	labels[labelOwner] = "helm"
	labels[labelStatus] = rel.Info.Status.String()
	labels[labelVersion] = strconv.Itoa(rel.Version)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       secretType,
		"data":       map[string]any{"release": base64.StdEncoding.EncodeToString([]byte(encoded))},
	}}
	obj.SetName(secretName(rel.Name, rel.Version))
	obj.SetNamespace(rel.Namespace)
	obj.SetLabels(labels)
	return obj, nil
}

func encodeRelease(rel *release.Release) (string, error) {
	raw, err := json.Marshal(rel)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodeRelease(data string) (*release.Release, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	if len(raw) > 3 && bytes.Equal(raw[:3], []byte{0x1f, 0x8b, 0x08}) {
		r, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		if raw, err = io.ReadAll(r); err != nil {
			return nil, err
		}
	}
	var rel release.Release
	if err := json.Unmarshal(raw, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

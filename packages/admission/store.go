package admission

import (
	"context"
	"encoding/base64"
	"encoding/json"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

type store struct {
	client *kine.Client
}

func decodeJSON[T any](data []byte) (T, error) {
	var v T
	return v, json.Unmarshal(data, &v)
}

type stored[T any] struct {
	key      string
	revision int64
	object   T
}

func listPrefix[T any](ctx context.Context, client *kine.Client, prefix string) ([]T, error) {
	items, err := listStored[T](ctx, client, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(items))
	for _, item := range items {
		out = append(out, item.object)
	}
	return out, nil
}

func listStored[T any](ctx context.Context, client *kine.Client, prefix string) ([]stored[T], error) {
	kvs, _, _, err := client.List(ctx, prefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make([]stored[T], 0, len(kvs))
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			return nil, err
		}
		v, err := decodeJSON[T](data)
		if err != nil {
			return nil, err
		}
		out = append(out, stored[T]{key: kv.Key, revision: kv.ModRevision, object: v})
	}
	return out, nil
}

func putJSON(ctx context.Context, client *kine.Client, key string, obj any, revision int64) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = client.Put(ctx, key, data, revision)
	return err
}

func getJSON[T any](ctx context.Context, client *kine.Client, key string) (T, bool, error) {
	var zero T
	kv, _, err := client.Get(ctx, key)
	if err == kine.ErrNotFound {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return zero, false, err
	}
	v, err := decodeJSON[T](data)
	return v, err == nil, err
}

func (s *store) validatingConfigs(ctx context.Context) ([]admissionregv1.ValidatingWebhookConfiguration, error) {
	return listPrefix[admissionregv1.ValidatingWebhookConfiguration](ctx, s.client, "/registry/validatingwebhookconfigurations/")
}

func (s *store) mutatingConfigs(ctx context.Context) ([]admissionregv1.MutatingWebhookConfiguration, error) {
	return listPrefix[admissionregv1.MutatingWebhookConfiguration](ctx, s.client, "/registry/mutatingwebhookconfigurations/")
}

func (s *store) policies(ctx context.Context) ([]admissionregv1.ValidatingAdmissionPolicy, error) {
	return listPrefix[admissionregv1.ValidatingAdmissionPolicy](ctx, s.client, "/registry/validatingadmissionpolicies/")
}

func (s *store) bindings(ctx context.Context) ([]admissionregv1.ValidatingAdmissionPolicyBinding, error) {
	return listPrefix[admissionregv1.ValidatingAdmissionPolicyBinding](ctx, s.client, "/registry/validatingadmissionpolicybindings/")
}

func (s *store) mutatingPolicies(ctx context.Context) ([]admissionregv1.MutatingAdmissionPolicy, error) {
	return listPrefix[admissionregv1.MutatingAdmissionPolicy](ctx, s.client, "/registry/mutatingadmissionpolicies/")
}

func (s *store) mutatingBindings(ctx context.Context) ([]admissionregv1.MutatingAdmissionPolicyBinding, error) {
	return listPrefix[admissionregv1.MutatingAdmissionPolicyBinding](ctx, s.client, "/registry/mutatingadmissionpolicybindings/")
}

func (s *store) service(ctx context.Context, ns, name string) (corev1.Service, bool, error) {
	return getJSON[corev1.Service](ctx, s.client, "/registry/services/"+ns+"/"+name)
}

func (s *store) endpoints(ctx context.Context, ns, name string) (corev1.Endpoints, bool, error) {
	return getJSON[corev1.Endpoints](ctx, s.client, "/registry/endpoints/"+ns+"/"+name)
}

func (s *store) endpointSlices(ctx context.Context, ns string) ([]discoveryv1.EndpointSlice, error) {
	return listPrefix[discoveryv1.EndpointSlice](ctx, s.client, "/registry/endpointslices/"+ns+"/")
}

func (s *store) pod(ctx context.Context, ns, name string) (corev1.Pod, bool, error) {
	return getJSON[corev1.Pod](ctx, s.client, "/registry/pods/"+ns+"/"+name)
}

func (s *store) node(ctx context.Context, name string) (corev1.Node, bool, error) {
	return getJSON[corev1.Node](ctx, s.client, "/registry/nodes/"+name)
}

func (s *store) namespace(ctx context.Context, name string) (corev1.Namespace, bool, error) {
	return getJSON[corev1.Namespace](ctx, s.client, "/registry/namespaces/"+name)
}

func (s *store) serviceAccount(ctx context.Context, ns, name string) (corev1.ServiceAccount, bool, error) {
	return getJSON[corev1.ServiceAccount](ctx, s.client, "/registry/serviceaccounts/"+ns+"/"+name)
}

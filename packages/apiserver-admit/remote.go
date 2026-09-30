package admit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
)

const Endpoint = "https://admission.internal/admit"

type Request struct {
	Phase       string                      `json:"phase"`
	Name        string                      `json:"name"`
	Namespace   string                      `json:"namespace"`
	Resource    schema.GroupVersionResource `json:"resource"`
	Subresource string                      `json:"subresource"`
	Operation   string                      `json:"operation"`
	DryRun      bool                        `json:"dryRun"`
	Kind        schema.GroupVersionKind     `json:"kind"`
	Object      map[string]any              `json:"object"`
	OldObject   map[string]any              `json:"oldObject"`
	User        User                        `json:"user"`
}

type User struct {
	Username string              `json:"username"`
	UID      string              `json:"uid"`
	Groups   []string            `json:"groups"`
	Extra    map[string][]string `json:"extra"`
}

type Response struct {
	Allowed bool           `json:"allowed"`
	Message string         `json:"message"`
	Reason  string         `json:"reason,omitempty"`
	Object  map[string]any `json:"object"`
}

type remote struct {
	client *http.Client
}

func New(client *http.Client) admission.Interface {
	return &remote{client: client}
}

var (
	_ admission.MutationInterface   = (*remote)(nil)
	_ admission.ValidationInterface = (*remote)(nil)
)

func (remote) Handles(op admission.Operation) bool {
	return op == admission.Create || op == admission.Update || op == admission.Delete || op == admission.Connect
}

func (r *remote) Admit(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if err := r.call(ctx, a, "admit"); err != nil {
		return err
	}
	if o != nil && a.GetObject() != nil {
		o.GetObjectDefaulter().Default(a.GetObject())
	}
	return r.call(ctx, a, "validate")
}

func (r *remote) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	return r.call(ctx, a, "validate")
}

func (r *remote) call(ctx context.Context, a admission.Attributes, phase string) error {
	obj, err := toMap(a.GetObject())
	if err != nil {
		return err
	}
	old, err := toMap(a.GetOldObject())
	if err != nil {
		return err
	}
	body, err := json.Marshal(Request{
		Phase:       phase,
		Name:        a.GetName(),
		Namespace:   a.GetNamespace(),
		Resource:    a.GetResource(),
		Subresource: a.GetSubresource(),
		Operation:   string(a.GetOperation()),
		DryRun:      a.IsDryRun(),
		Kind:        a.GetKind(),
		Object:      obj,
		OldObject:   old,
		User:        userFrom(a.GetUserInfo()),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return admission.NewForbidden(a, fmt.Errorf("admission: %w", err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return admission.NewForbidden(a, fmt.Errorf("admission: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		return admission.NewForbidden(a, fmt.Errorf("admission: HTTP %d: %s", resp.StatusCode, string(data)))
	}
	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return admission.NewForbidden(a, fmt.Errorf("admission: %w", err))
	}
	if !out.Allowed {
		msg := out.Message
		if msg == "" {
			msg = "denied"
		}
		if out.Reason == string(metav1.StatusReasonInvalid) {
			gk := schema.GroupKind{Group: a.GetKind().Group, Kind: a.GetKind().Kind}
			return apierrors.NewInvalid(gk, a.GetName(), field.ErrorList{field.Invalid(field.NewPath("spec"), "", msg)})
		}
		if out.Reason == string(metav1.StatusReasonInternalError) {
			return apierrors.NewInternalError(fmt.Errorf("%s", msg))
		}
		return admission.NewForbidden(a, fmt.Errorf("%s", msg))
	}
	if phase == "admit" && out.Object != nil && a.GetObject() != nil {
		return fromMap(out.Object, a.GetObject())
	}
	return nil
}

func userFrom(info user.Info) User {
	if info == nil {
		return User{}
	}
	return User{Username: info.GetName(), UID: info.GetUID(), Groups: info.GetGroups(), Extra: info.GetExtra()}
}

func toMap(obj runtime.Object) (map[string]any, error) {
	if obj == nil {
		return nil, nil
	}
	return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
}

func fromMap(m map[string]any, obj runtime.Object) error {
	return runtime.DefaultUnstructuredConverter.FromUnstructured(m, obj)
}

package core

import (
	"context"
	"fmt"
	"time"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	utilruntime.Must(authenticationv1.AddToScheme(scheme.Scheme))
	registry.Subresources["serviceaccounts/token"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return &saTokenREST{sas: stores["serviceaccounts"], pods: stores["pods"], secrets: stores["secrets"], nodes: stores["nodes"], hmac: deps.TokenHMAC}
	}
}

type saTokenREST struct {
	sas     rest.Getter
	pods    rest.Getter
	secrets rest.Getter
	nodes   rest.Getter
	hmac    []byte
}

var (
	_ rest.NamedCreater             = (*saTokenREST)(nil)
	_ rest.GroupVersionKindProvider = (*saTokenREST)(nil)
	_ rest.Scoper                   = (*saTokenREST)(nil)
	_ rest.SingularNameProvider     = (*saTokenREST)(nil)
)

func (saTokenREST) New() runtime.Object { return &authenticationv1.TokenRequest{} }
func (saTokenREST) Destroy()            {}
func (saTokenREST) NamespaceScoped() bool {
	return true
}
func (saTokenREST) GetSingularName() string { return "tokenrequest" }
func (saTokenREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return authenticationv1.SchemeGroupVersion.WithKind("TokenRequest")
}

const (
	minTokenExpirationSeconds     = 10 * 60
	maxTokenExpirationSeconds     = 1 << 32
	defaultTokenExpirationSeconds = 60 * 60
)

func (r *saTokenREST) Create(ctx context.Context, name string, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	req, ok := obj.(*authenticationv1.TokenRequest)
	if !ok {
		return nil, apierrors.NewBadRequest("not a TokenRequest")
	}
	if r.sas == nil {
		return nil, apierrors.NewInternalError(errNoServiceAccounts)
	}
	if len(r.hmac) == 0 {
		return nil, apierrors.NewServiceUnavailable("service account tokens are not configured")
	}
	namespace, ok := genericapirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("namespace is required")
	}
	gk := authenticationv1.SchemeGroupVersion.WithKind("TokenRequest").GroupKind()
	if req.Name != "" && req.Name != name {
		return nil, apierrors.NewInvalid(gk, name, field.ErrorList{field.Invalid(field.NewPath("metadata").Child("name"), req.Name, "must match the service account name if specified")})
	}
	if req.Namespace != "" && req.Namespace != namespace {
		return nil, apierrors.NewInvalid(gk, name, field.ErrorList{field.Invalid(field.NewPath("metadata").Child("namespace"), req.Namespace, "must match the service account namespace if specified")})
	}
	got, err := r.sas.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	sa, ok := got.(*corev1.ServiceAccount)
	if !ok {
		return nil, apierrors.NewInternalError(errNoServiceAccounts)
	}
	if req.UID != "" && req.UID != sa.UID {
		return nil, apierrors.NewConflict(schema.GroupResource{Group: gk.Group, Resource: gk.Kind}, name, fmt.Errorf("the UID in the token request (%s) does not match the UID of the service account (%s)", req.UID, sa.UID))
	}
	secs := int64(defaultTokenExpirationSeconds)
	if req.Spec.ExpirationSeconds != nil {
		secs = *req.Spec.ExpirationSeconds
	}
	if secs < minTokenExpirationSeconds || secs > maxTokenExpirationSeconds {
		return nil, apierrors.NewInvalid(gk, name, field.ErrorList{field.Invalid(field.NewPath("spec").Child("expirationSeconds"), secs, fmt.Sprintf("must be between %d and %d seconds", minTokenExpirationSeconds, maxTokenExpirationSeconds))})
	}
	audiences := req.Spec.Audiences
	if len(audiences) == 0 {
		audiences = auth.APIAudiences
	}
	bound, err := r.boundObjects(ctx, name, namespace, req.Spec.BoundObjectRef)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	exp := now.Add(time.Duration(secs) * time.Second)
	token, err := auth.IssueServiceAccountToken(r.hmac, sa.Namespace, sa.Name, string(sa.UID), exp, audiences, bound)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	out := req.DeepCopy()
	out.Name = sa.Name
	out.Namespace = sa.Namespace
	out.UID = sa.UID
	out.CreationTimestamp = metav1.NewTime(now)
	out.Spec.Audiences = audiences
	out.Spec.ExpirationSeconds = &secs
	out.Status = authenticationv1.TokenRequestStatus{Token: token, ExpirationTimestamp: metav1.NewTime(exp)}
	return out, nil
}

func (r *saTokenREST) boundObjects(ctx context.Context, saName, namespace string, ref *authenticationv1.BoundObjectReference) (auth.BoundObjects, error) {
	var bound auth.BoundObjects
	if ref == nil {
		return bound, nil
	}
	gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
	var uid types.UID
	switch {
	case gvk.Group == "" && gvk.Kind == "Pod":
		obj, err := r.pods.Get(ctx, ref.Name, &metav1.GetOptions{})
		if err != nil {
			return bound, err
		}
		pod := obj.(*corev1.Pod)
		if pod.Spec.ServiceAccountName != saName {
			return bound, apierrors.NewBadRequest(fmt.Sprintf("cannot bind token for serviceaccount %q to pod running with different serviceaccount name.", saName))
		}
		uid = pod.UID
		bound.Pod = &auth.BoundObject{Name: pod.Name, UID: string(pod.UID)}
		if pod.Spec.NodeName != "" {
			bound.Node = &auth.BoundObject{Name: pod.Spec.NodeName}
			if nodeObj, err := r.nodes.Get(genericapirequest.WithNamespace(ctx, ""), pod.Spec.NodeName, &metav1.GetOptions{}); err == nil {
				bound.Node.UID = string(nodeObj.(*corev1.Node).UID)
			} else if !apierrors.IsNotFound(err) {
				return bound, apierrors.NewInternalError(err)
			}
		}
	case gvk.Group == "" && gvk.Kind == "Node":
		obj, err := r.nodes.Get(genericapirequest.WithNamespace(ctx, ""), ref.Name, &metav1.GetOptions{})
		if err != nil {
			return bound, err
		}
		node := obj.(*corev1.Node)
		uid = node.UID
		bound.Node = &auth.BoundObject{Name: node.Name, UID: string(node.UID)}
	case gvk.Group == "" && gvk.Kind == "Secret":
		obj, err := r.secrets.Get(ctx, ref.Name, &metav1.GetOptions{})
		if err != nil {
			return bound, err
		}
		secret := obj.(*corev1.Secret)
		uid = secret.UID
		bound.Secret = &auth.BoundObject{Name: secret.Name, UID: string(secret.UID)}
	default:
		return bound, apierrors.NewBadRequest(fmt.Sprintf("cannot bind token to object of type %s", gvk.String()))
	}
	if ref.UID != "" && uid != ref.UID {
		return auth.BoundObjects{}, apierrors.NewConflict(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, ref.Name, fmt.Errorf("the UID in the bound object reference (%s) does not match the UID in record. The object might have been deleted and then recreated", ref.UID))
	}
	return bound, nil
}

var errNoServiceAccounts = errString("serviceaccounts store is missing")

type errString string

func (e errString) Error() string { return string(e) }

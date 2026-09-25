package core

import (
	"context"
	"time"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	utilruntime.Must(authenticationv1.AddToScheme(scheme.Scheme))
	registry.Subresources["serviceaccounts/token"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return &saTokenREST{sas: stores["serviceaccounts"], hmac: deps.TokenHMAC}
	}
}

type saTokenREST struct {
	sas  *registry.Store
	hmac []byte
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
	got, err := r.sas.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	sa, ok := got.(*corev1.ServiceAccount)
	if !ok {
		return nil, apierrors.NewInternalError(errNoServiceAccounts)
	}
	secs := int64(3600)
	exp := time.Now().Add(time.Hour)
	if req.Spec.ExpirationSeconds != nil && *req.Spec.ExpirationSeconds > 0 {
		secs = *req.Spec.ExpirationSeconds
		exp = time.Now().Add(time.Duration(secs) * time.Second)
	}
	token, err := auth.IssueServiceAccountToken(r.hmac, sa.Namespace, sa.Name, string(sa.UID), exp, req.Spec.Audiences)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	out := req.DeepCopy()
	out.Name = name
	out.Namespace = sa.Namespace
	out.Spec.ExpirationSeconds = &secs
	out.Status = authenticationv1.TokenRequestStatus{Token: token, ExpirationTimestamp: metav1.NewTime(exp)}
	return out, nil
}

var errNoServiceAccounts = errString("serviceaccounts store is missing")

type errString string

func (e errString) Error() string { return string(e) }

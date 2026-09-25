package certificates

import (
	"context"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	registry.Customizers["certificatesigningrequests"] = func(store *registry.Store, _ registry.Deps) {
		store.CreateStrategy = csrCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = csrUpdateStrategy{store.UpdateStrategy}
	}
}

type csrCreateStrategy struct{ rest.RESTCreateStrategy }

func (s csrCreateStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if s.RESTCreateStrategy != nil {
		s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	}
	csr, ok := obj.(*certificatesv1.CertificateSigningRequest)
	if !ok {
		return
	}
	csr.Spec.Username = ""
	csr.Spec.UID = ""
	csr.Spec.Groups = nil
	csr.Spec.Extra = nil
	if user, ok := genericapirequest.UserFrom(ctx); ok {
		csr.Spec.Username = user.GetName()
		csr.Spec.UID = user.GetUID()
		csr.Spec.Groups = user.GetGroups()
		if extra := user.GetExtra(); len(extra) > 0 {
			csr.Spec.Extra = make(map[string]certificatesv1.ExtraValue, len(extra))
			for k, v := range extra {
				csr.Spec.Extra[k] = v
			}
		}
	}
	csr.Status = certificatesv1.CertificateSigningRequestStatus{}
}

type csrUpdateStrategy struct{ rest.RESTUpdateStrategy }

func (s csrUpdateStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if s.RESTUpdateStrategy != nil {
		s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	}
	newCSR, ok1 := obj.(*certificatesv1.CertificateSigningRequest)
	oldCSR, ok2 := old.(*certificatesv1.CertificateSigningRequest)
	if !ok1 || !ok2 {
		return
	}
	newCSR.Spec = oldCSR.Spec
	newCSR.Status = oldCSR.Status
}

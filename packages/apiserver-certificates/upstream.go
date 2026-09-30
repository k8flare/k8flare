package certificates

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/certificates"
	csr "k8s.io/kubernetes/pkg/registry/certificates/certificates"
)

func init() {
	utilruntime.Must(certificates.AddToScheme(registry.InternalScheme))
	utilruntime.Must(certificatesv1.AddToScheme(registry.InternalScheme))
	registry.Upstreams[schema.GroupResource{Group: "certificates.k8s.io", Resource: "certificatesigningrequests"}] = registry.Upstream{Strategy: csr.Strategy, Status: csr.StatusStrategy}
}

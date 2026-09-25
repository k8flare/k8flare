package customresources

import (
	"context"
	jsonly "github.com/k8flare/k8flare/packages/apiserver-jsonly"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"github.com/k8flare/k8flare/packages/crdreconcile"
	apiextensionshelpers "k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	clientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	listers "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/establish"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresourcedefinition"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/apiserver/pkg/registry/generic"
	registryrest "k8s.io/apiserver/pkg/registry/rest"
	kmux "k8s.io/apiserver/pkg/server/mux"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/apiserver/pkg/util/webhook"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-openapi/pkg/handler3"
)

type Config struct {
	Kine      *http.Client
	Admission *http.Client
	Tunnel    *http.Client
	Outbound  *http.Client
	Hooks     *http.Client
}

var apiGroup = metav1.APIGroup{
	Name:             apiextensionsv1.GroupName,
	Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: apiextensionsv1.SchemeGroupVersion.String(), Version: apiextensionsv1.SchemeGroupVersion.Version}},
	PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: apiextensionsv1.SchemeGroupVersion.String(), Version: apiextensionsv1.SchemeGroupVersion.Version},
}

func NewHandler(cfg Config) (http.Handler, error) {
	scheme, codecs := apiextensionsapiserver.Scheme, apiextensionsapiserver.Codecs
	client := &kine.Client{HTTP: cfg.Kine}
	crdREST, err := customresourcedefinition.NewREST(scheme, restOptions{client: client, codec: codecs.LegacyCodec(apiextensionsv1.SchemeGroupVersion)})
	if err != nil {
		return nil, err
	}
	crdMux := http.NewServeMux()
	container := restful.NewContainer()
	container.ServeMux = crdMux
	container.Router(restful.CurlyRouter{})
	group := &endpoints.APIGroupVersion{
		Storage: map[string]registryrest.Storage{
			"customresourcedefinitions":        crdREST,
			"customresourcedefinitions/status": customresourcedefinition.NewStatusREST(scheme, crdREST),
		},
		Root:                       "/apis",
		GroupVersion:               apiextensionsv1.SchemeGroupVersion,
		MetaGroupVersion:           &metav1.SchemeGroupVersion,
		Creater:                    scheme,
		Convertor:                  scheme,
		Typer:                      scheme,
		Defaulter:                  scheme,
		ConvertabilityChecker:      scheme,
		UnsafeConvertor:            runtime.UnsafeObjectConvertor(scheme),
		Namer:                      runtime.Namer(meta.NewAccessor()),
		Serializer:                 jsonly.JSONOnly{NegotiatedSerializer: codecs},
		ParameterCodec:             metav1.ParameterCodec,
		EquivalentResourceRegistry: runtime.NewEquivalentResourceRegistry(),
		TypeConverter:              managedfields.NewDeducedTypeConverter(),
		Admit:                      crdAdmit(cfg.Admission),
		MinRequestTimeout:          30 * time.Minute,
	}
	if _, _, err := group.InstallREST(container); err != nil {
		return nil, err
	}
	container.Add(discovery.NewAPIGroupHandler(codecs, apiGroup).WebService())

	mux := http.NewServeMux()
	handler := auth.WithRemoteUser(mux)
	crdClient, err := clientset.NewForConfig(&rest.Config{Host: "https://customresources.internal", Transport: loopback{handler},
		// The control plane serves JSON only; client-go would otherwise default
		// to protobuf and the condition controllers could not write status.
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"}})
	if err != nil {
		return nil, err
	}
	factory := informers.NewSharedInformerFactory(crdClient, 5*time.Minute)
	refillable := registerRefillable(factory)
	crdInformer := factory.Apiextensions().V1().CustomResourceDefinitions()
	versionDiscovery, groupDiscovery := apiextensionsapiserver.NewDiscoveryHandlers(http.NotFoundHandler())
	establishing := establish.NewEstablishingController(crdInformer, crdClient.ApiextensionsV1())
	crdHandler, err := apiextensionsapiserver.NewCustomResourceDefinitionHandler(
		versionDiscovery, groupDiscovery, crdInformer, http.NotFoundHandler(), restOptions{client: client, codec: unstructured.UnstructuredJSONScheme},
		crdAdmit(cfg.Admission), establishing, webhook.NewDefaultServiceResolver(), newConversionResolver(client, cfg.Tunnel, cfg.Outbound, cfg.Hooks).install(), 1,
		authorizerfactory.NewAlwaysAllowAuthorizer(), 60*time.Second, 30*time.Minute, staticOpenAPISpec(), 3*1024*1024)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	discoveryController := apiextensionsapiserver.NewDiscoveryController(crdInformer, versionDiscovery, groupDiscovery, nil)
	discoverySynced := make(chan struct{})
	go discoveryController.Run(ctx.Done(), discoverySynced)

	openAPIV3Service := handler3.NewOpenAPIService()
	openAPIV3Mux := kmux.NewPathRecorderMux("customresources-openapi")
	if err := openAPIV3Service.RegisterOpenAPIV3VersionedService("/openapi/v3", openAPIV3Mux); err != nil {
		return nil, err
	}
	fresh := freshCRDs{client: client, informer: refillable, reconciler: &reconciler{deps: crdreconcile.Deps{
		Client:  crdClient,
		Kine:    client,
		Drained: func() bool { return queueWork.drained(crdreconcile.QueueNames...) },
	}}}
	pub := &crdOpenAPI{svc: openAPIV3Service, informer: refillable}
	crdWrites := admitCRDWrites(crdAdmit(cfg.Admission), crdMux)
	mux.Handle("/apis/apiextensions.k8s.io/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bypassCRDGate(r) {
			crdWrites.ServeHTTP(w, r)
			return
		}
		fresh.gate(crdWrites).ServeHTTP(w, r)
	}))
	mux.Handle("/apis", fresh.gate(wrapCRDAggregated(crdInformer.Lister(), rootAPIs(crdInformer.Lister(), codecs))))
	mux.Handle("/apis/", fresh.gate(afterSync(discoverySynced, crdHandler)))
	mux.Handle("/openapi/v2", pub.serveV2(fresh))
	mux.Handle("/openapi/v3", pub.handler(fresh, openAPIV3Mux))
	mux.Handle("/openapi/v3/", pub.handler(fresh, openAPIV3Mux))
	return handler, nil
}

func crdAdmit(client *http.Client) admission.Interface {
	if client == nil {
		return admission.NewChainHandler()
	}
	return admit.New(client)
}

type restOptions struct {
	client *kine.Client
	codec  runtime.Codec
}

func (o restOptions) GetRESTOptions(gr schema.GroupResource, _ runtime.Object) (generic.RESTOptions, error) {
	prefix := gr.Resource
	if gr.Group != "" {
		prefix = gr.Group + "/" + gr.Resource
	}
	return generic.RESTOptions{
		StorageConfig: &storagebackend.ConfigForResource{
			Config:        storagebackend.Config{Codec: o.codec},
			GroupResource: gr,
		},
		Decorator:               o.decorate,
		ResourcePrefix:          prefix,
		DeleteCollectionWorkers: 1,
		EnableGarbageCollection: true,
	}, nil
}

func (o restOptions) decorate(config *storagebackend.ConfigForResource, _ string, _ func(runtime.Object) (string, error), newFunc, _ func() runtime.Object, _ storage.AttrFunc, _ storage.IndexerFuncs, _ *cache.Indexers) (storage.Interface, factory.DestroyFunc, error) {
	return kine.NewStorage(o.client, config.Codec, newFunc), func() {}, nil
}

func afterSync(synced <-chan struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-synced:
			next.ServeHTTP(w, r)
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
			http.Error(w, "custom resource definitions not synced yet", http.StatusServiceUnavailable)
		}
	})
}

func rootAPIs(lister listers.CustomResourceDefinitionLister, codecs runtime.NegotiatedSerializer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crds, err := lister.List(labels.Everything())
		if err != nil {
			responsewriters.InternalError(w, r, err)
			return
		}
		byName := map[string]*metav1.APIGroup{}
		for _, crd := range crds {
			if !apiextensionshelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established) {
				continue
			}
			g := byName[crd.Spec.Group]
			if g == nil {
				g = &metav1.APIGroup{Name: crd.Spec.Group}
				byName[crd.Spec.Group] = g
			}
			for _, v := range crd.Spec.Versions {
				if !v.Served {
					continue
				}
				gv := metav1.GroupVersionForDiscovery{GroupVersion: crd.Spec.Group + "/" + v.Name, Version: v.Name}
				if !containsVersion(g.Versions, gv) {
					g.Versions = append(g.Versions, gv)
				}
			}
		}
		list := &metav1.APIGroupList{Groups: []metav1.APIGroup{apiGroup}}
		names := make([]string, 0, len(byName))
		for name := range byName {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			g := byName[name]
			if len(g.Versions) == 0 {
				continue
			}
			g.PreferredVersion = g.Versions[0]
			list.Groups = append(list.Groups, *g)
		}
		responsewriters.WriteObjectNegotiated(codecs, negotiation.DefaultEndpointRestrictions, schema.GroupVersion{}, w, r, http.StatusOK, list, false)
	})
}

func containsVersion(list []metav1.GroupVersionForDiscovery, gv metav1.GroupVersionForDiscovery) bool {
	for _, v := range list {
		if v == gv {
			return true
		}
	}
	return false
}

const crdStoragePrefix = "/registry/apiextensions.k8s.io/customresourcedefinitions/"

type freshCRDs struct {
	client     *kine.Client
	informer   *refillableInformer
	reconciler *reconciler
}

func (f freshCRDs) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CRD-Instance", instanceID)
		kvs, _, _, err := f.client.List(r.Context(), crdStoragePrefix, "", 0)
		if err == nil && f.reconciler.run(r.Context(), decodeCRDs(kvs)) {
			kvs, _, _, err = f.client.List(r.Context(), crdStoragePrefix, "", 0)
		}
		if err == nil {
			n := f.informer.refill(kvs)
			if n > 0 {
				settleDiscovery(r.Context())
			}
			refillHeader(w, n)
			pending, finalize := pendingWork(decodeCRDs(kvs))
			count := len(finalize)
			if pending {
				count++
			}
			w.Header().Set("X-CRD-Pending", strconv.Itoa(count))
		}
		if name := crdNameForPath(r.URL.Path); name != "" {
			if obj, exists, _ := f.informer.GetIndexer().GetByKey(name); exists {
				w.Header().Set("X-CRD-RV", obj.(*apiextensionsv1.CustomResourceDefinition).ResourceVersion)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func bypassCRDGate(r *http.Request) bool {
	if strings.Contains(r.URL.Path, "/status") {
		return true
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func crdNameForPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "apis" {
		return ""
	}
	plural := parts[3]
	if plural == "namespaces" && len(parts) >= 6 {
		plural = parts[5]
	}
	return plural + "." + parts[1]
}

package customresources

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	apiextensionshelpers "k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	clientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	listers "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/establish"
	"k8s.io/apiextensions-apiserver/pkg/controller/finalizer"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapiv3"
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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-openapi/pkg/handler3"
)

type Config struct {
	Kine *http.Client
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
	mux := http.NewServeMux()
	container := restful.NewContainer()
	container.ServeMux = mux
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
		Serializer:                 codecs,
		ParameterCodec:             metav1.ParameterCodec,
		EquivalentResourceRegistry: runtime.NewEquivalentResourceRegistry(),
		TypeConverter:              managedfields.NewDeducedTypeConverter(),
		Admit:                      admission.NewChainHandler(),
		MinRequestTimeout:          30 * time.Minute,
	}
	if _, _, err := group.InstallREST(container); err != nil {
		return nil, err
	}
	container.Add(discovery.NewAPIGroupHandler(codecs, apiGroup).WebService())

	handler := auth.WithRemoteUser(mux)
	crdClient, err := clientset.NewForConfig(&rest.Config{Host: "https://customresources.internal", Transport: loopback{handler}})
	if err != nil {
		return nil, err
	}
	factory := informers.NewSharedInformerFactory(crdClient, 5*time.Minute)
	crdInformer := factory.Apiextensions().V1().CustomResourceDefinitions()
	versionDiscovery, groupDiscovery := apiextensionsapiserver.NewDiscoveryHandlers(http.NotFoundHandler())
	establishing := establish.NewEstablishingController(crdInformer, crdClient.ApiextensionsV1())
	crdHandler, err := apiextensionsapiserver.NewCustomResourceDefinitionHandler(
		versionDiscovery, groupDiscovery, crdInformer, http.NotFoundHandler(), restOptions{client: client, codec: unstructured.UnstructuredJSONScheme},
		admission.NewChainHandler(), establishing, nil, nil, 1,
		authorizerfactory.NewAlwaysAllowAuthorizer(), 60*time.Second, 30*time.Minute, nil, 3*1024*1024)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	discoveryController := apiextensionsapiserver.NewDiscoveryController(crdInformer, versionDiscovery, groupDiscovery, nil)
	finalizing := finalizer.NewCRDFinalizer(crdInformer, crdClient.ApiextensionsV1(), crdHandler)
	factory.Start(ctx.Done())
	go establishing.RunWithContext(ctx)
	go finalizing.RunWithContext(5, ctx)
	discoverySynced := make(chan struct{})
	go discoveryController.Run(ctx.Done(), discoverySynced)

	openAPIV3Service := handler3.NewOpenAPIService()
	openAPIV3Mux := kmux.NewPathRecorderMux("customresources-openapi")
	if err := openAPIV3Service.RegisterOpenAPIV3VersionedService("/openapi/v3", openAPIV3Mux); err != nil {
		return nil, err
	}
	openAPIV3Controller := openapiv3.NewController(crdInformer)
	go openAPIV3Controller.Run(openAPIV3Service, ctx.Done())

	fresh := freshCRDs{client: client, lister: crdInformer.Lister()}
	mux.Handle("/apis", fresh.gate(rootAPIs(crdInformer.Lister(), codecs)))
	mux.Handle("/apis/", fresh.gate(afterSync(discoverySynced, crdHandler)))
	mux.Handle("/openapi/v3", openAPIV3Mux)
	mux.Handle("/openapi/v3/", openAPIV3Mux)
	return handler, nil
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

const (
	crdStoragePrefix = "/registry/apiextensions.k8s.io/customresourcedefinitions/"
	freshWait        = 5 * time.Second
	freshPoll        = 200 * time.Millisecond
)

type freshCRDs struct {
	client *kine.Client
	lister listers.CustomResourceDefinitionLister
}

func (f freshCRDs) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDiscoveryPath(r.URL.Path) {
			f.waitFresh(r.Context())
		}
		next.ServeHTTP(w, r)
	})
}

func isDiscoveryPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) <= 3 && parts[0] == "apis"
}

func (f freshCRDs) waitFresh(ctx context.Context) {
	kvs, _, _, err := f.client.List(ctx, crdStoragePrefix, "", 0)
	if err != nil {
		return
	}
	want := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		want[strings.TrimPrefix(kv.Key, crdStoragePrefix)] = strconv.FormatInt(kv.ModRevision, 10)
	}
	deadline := time.Now().Add(freshWait)
	for {
		if f.matches(want) {
			return
		}
		if time.Now().After(deadline) {
			println("customresources: discovery served from a stale CRD cache")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(freshPoll):
		}
	}
}

func (f freshCRDs) matches(want map[string]string) bool {
	crds, err := f.lister.List(labels.Everything())
	if err != nil || len(crds) != len(want) {
		return false
	}
	for _, crd := range crds {
		if rv, ok := want[crd.Name]; !ok || rv != crd.ResourceVersion {
			return false
		}
	}
	return true
}

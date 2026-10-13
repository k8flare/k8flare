package apiregistration

import (
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

func apiServiceStore(t *testing.T) *registry.Store {
	t.Helper()
	return registrytest.Store(t, schema.GroupVersion{Group: "apiregistration.k8s.io", Version: "v1"}, metav1.APIResource{Name: "apiservices", Kind: "APIService"})
}

func validAPIService() *apiregistrationv1.APIService {
	return &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: "v1.example.com"},
		Spec: apiregistrationv1.APIServiceSpec{
			Group:                "example.com",
			Version:              "v1",
			GroupPriorityMinimum: 1000,
			VersionPriority:      15,
		},
	}
}

func TestAPIServiceRejectsMismatchedName(t *testing.T) {
	obj := validAPIService()
	obj.Name = "v2.example.com"
	err := registrytest.Create(apiServiceStore(t), obj)
	registrytest.RequireFieldError(t, err, "metadata.name", "must be `spec.version+\".\"+spec.group`")
}

func TestAPIServiceRejectsOutOfRangeGroupPriorityMinimum(t *testing.T) {
	obj := validAPIService()
	obj.Spec.GroupPriorityMinimum = 20001
	err := registrytest.Create(apiServiceStore(t), obj)
	registrytest.RequireFieldError(t, err, "spec.groupPriorityMinimum", "must be positive and less than 20000")
}

func TestAPIServiceRejectsLocalCABundle(t *testing.T) {
	obj := validAPIService()
	obj.Spec.CABundle = []byte("ca")
	err := registrytest.Create(apiServiceStore(t), obj)
	registrytest.RequireFieldError(t, err, "spec.caBundle", "local APIServices may not have a caBundle")
}

func TestAPIServiceUpdateRejectsOutOfRangeVersionPriority(t *testing.T) {
	old := validAPIService()
	old.ResourceVersion = "1"
	obj := old.DeepCopy()
	obj.Spec.VersionPriority = 1001
	err := registrytest.Update(apiServiceStore(t), obj, old)
	registrytest.RequireFieldError(t, err, "spec.versionPriority", "must be positive and less than 1000")
}

func TestAPIServiceStatusUpdateRejectsUnknownConditionStatus(t *testing.T) {
	store := apiServiceStore(t)
	old := validAPIService()
	old.ResourceVersion = "1"
	obj := old.DeepCopy()
	obj.Status.Conditions = []apiregistrationv1.APIServiceCondition{{Type: apiregistrationv1.Available, Status: "Maybe"}}
	strategy := registry.StatusStrategyFor(store)
	ctx := registrytest.Context(store)
	strategy.PrepareForUpdate(ctx, obj, old)
	err := rest.BeforeUpdate(strategy, ctx, obj, old)
	registrytest.RequireFieldError(t, err, "status.conditions[0].status", "Unsupported value")
}

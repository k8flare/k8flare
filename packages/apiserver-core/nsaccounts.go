package core

import (
	"context"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
)

var nsAccounts = struct {
	sas  *registry.Store
	cms  *registry.Store
	kine *kine.Client
}{}

func bindNamespaceAccounts(stores map[string]*registry.Store, client *kine.Client) {
	nsAccounts.sas = stores["serviceaccounts"]
	nsAccounts.cms = stores["configmaps"]
	nsAccounts.kine = client
}

func namespaceBeginCreate(ctx context.Context, obj runtime.Object, options *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
	name := obj.(*corev1.Namespace).Name
	dry := options != nil && len(options.DryRun) > 0
	return func(ctx context.Context, success bool) {
		if !success || dry || name == "" {
			return
		}
		provisionNamespaceAccounts(ctx, name)
	}, nil
}

func provisionNamespaceAccounts(ctx context.Context, name string) {
	if nsAccounts.sas != nil {
		create(genericapirequest.WithNamespace(ctx, name), nsAccounts.sas, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: name},
		})
	}
	if nsAccounts.cms == nil {
		return
	}
	ca := ""
	if nsAccounts.kine != nil {
		if pem, err := supervisor.NewVault(nsAccounts.kine).CAPEM(ctx, "server-ca"); err == nil {
			ca = string(pem)
		}
	}
	create(genericapirequest.WithNamespace(ctx, name), nsAccounts.cms, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: name},
		Data:       map[string]string{"ca.crt": ca},
	})
	if name == metav1.NamespaceSystem {
		ensureExtensionAuth(ctx)
	}
}

func ensureExtensionAuth(ctx context.Context) {
	if nsAccounts.cms == nil {
		return
	}
	ca := ""
	if nsAccounts.kine != nil {
		if pem, err := supervisor.NewVault(nsAccounts.kine).CAPEM(ctx, "server-ca"); err == nil {
			ca = string(pem)
		}
	}
	create(genericapirequest.WithNamespace(ctx, metav1.NamespaceSystem), nsAccounts.cms, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "extension-apiserver-authentication", Namespace: metav1.NamespaceSystem},
		Data: map[string]string{
			"client-ca-file":                     ca,
			"requestheader-client-ca-file":       ca,
			"requestheader-username-headers":     "X-Remote-User",
			"requestheader-group-headers":        "X-Remote-Group",
			"requestheader-extra-headers-prefix": "X-Remote-Extra-",
			"requestheader-allowed-names":        "",
		},
	})
}

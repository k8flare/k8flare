package core

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"maps"
	"slices"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/cert"
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
	if err := reconcileExtensionAuth(ctx); err != nil {
		println("apiserver: extension authentication:", err.Error())
	}
}

func reconcileExtensionAuth(ctx context.Context) error {
	if nsAccounts.cms == nil || nsAccounts.kine == nil {
		return nil
	}
	vault := supervisor.NewVault(nsAccounts.kine)
	clientCA, err := vault.CAPEM(ctx, "client-ca")
	if err != nil {
		return err
	}
	requestHeaderCA, err := vault.CAPEM(ctx, supervisor.RequestHeaderCAName)
	if err != nil {
		return err
	}
	serverCA, err := vault.CAPEM(ctx, "server-ca")
	if err != nil {
		return err
	}
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "extension-apiserver-authentication", Namespace: metav1.NamespaceSystem},
		Data: map[string]string{
			"client-ca-file":                     string(clientCA),
			"requestheader-client-ca-file":       string(requestHeaderCA),
			"requestheader-username-headers":     `["X-Remote-User"]`,
			"requestheader-group-headers":        `["X-Remote-Group"]`,
			"requestheader-extra-headers-prefix": `["X-Remote-Extra-"]`,
			"requestheader-allowed-names":        `["` + supervisor.RequestHeaderCN + `"]`,
		},
	}
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceSystem)
	got, err := nsAccounts.cms.Get(ctx, desired.Name, &metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = nsAccounts.cms.Create(ctx, desired, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	current := got.(*corev1.ConfigMap)
	updated := current.DeepCopy()
	if updated.Data == nil {
		updated.Data = map[string]string{}
	}
	for key, value := range desired.Data {
		if key == "client-ca-file" || key == "requestheader-client-ca-file" {
			value, err = mergeExtensionAuthCAs(current.Data[key], value, string(serverCA))
		} else {
			var existing, required []string
			_ = json.Unmarshal([]byte(value), &required)
			if old := current.Data[key]; old != "" {
				err = json.Unmarshal([]byte(old), &existing)
				if old == required[0] {
					existing, err = []string{old}, nil
				}
			}
			merged := []string{}
			for _, entry := range append(existing, required...) {
				if !slices.Contains(merged, entry) {
					merged = append(merged, entry)
				}
			}
			encoded, _ := json.Marshal(merged)
			value = string(encoded)
		}
		if err != nil {
			return err
		}
		updated.Data[key] = value
	}
	if maps.Equal(current.Data, updated.Data) {
		return nil
	}
	_, _, err = nsAccounts.cms.Update(ctx, updated.Name, rest.DefaultUpdatedObjectInfo(updated), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	return err
}

func mergeExtensionAuthCAs(existing, required, serverCA string) (string, error) {
	serverCerts, err := cert.ParseCertsPEM([]byte(serverCA))
	if err != nil {
		return "", err
	}
	excluded := map[string]bool{}
	for _, ca := range serverCerts {
		excluded[string(ca.Raw)] = true
	}
	seen := map[string]bool{}
	bundle := []byte{}
	for _, source := range []string{existing, required} {
		if source == "" {
			continue
		}
		certs, err := cert.ParseCertsPEM([]byte(source))
		if err != nil {
			return "", err
		}
		for _, ca := range certs {
			key := string(ca.Raw)
			if excluded[key] || seen[key] || !ca.NotAfter.After(time.Now().Add(-5*time.Minute)) {
				continue
			}
			seen[key] = true
			bundle = append(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})...)
		}
	}
	return string(bundle), nil
}

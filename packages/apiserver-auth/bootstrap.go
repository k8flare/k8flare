package auth

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/kubernetes/plugin/pkg/auth/authenticator/token/bootstrap"
)

type BootstrapSecrets interface {
	Secret(context.Context, string, string) (*corev1.Secret, error)
}

type BootstrapToken struct {
	Objects BootstrapSecrets
}

func (b BootstrapToken) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	return bootstrap.NewTokenAuthenticator(bootstrapSecretLister{ctx: ctx, objects: b.Objects}).AuthenticateToken(ctx, token)
}

type bootstrapSecretLister struct {
	ctx     context.Context
	objects BootstrapSecrets
}

func (l bootstrapSecretLister) Get(name string) (*corev1.Secret, error) {
	if l.objects == nil {
		return nil, fmt.Errorf("bootstrap token secret storage is unavailable")
	}
	secret, err := l.objects.Secret(l.ctx, metav1.NamespaceSystem, name)
	if storage.IsNotFound(err) {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, name)
	}
	return secret, err
}

func (l bootstrapSecretLister) List(labels.Selector) ([]*corev1.Secret, error) {
	return nil, fmt.Errorf("bootstrap token authentication does not list secrets")
}

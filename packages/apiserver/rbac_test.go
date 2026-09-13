//go:build !js

package apiserver_test

import (
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestRBAC(t *testing.T) {
	base, admin := startDevURL(t)
	c := ctx(t)

	readonly, err := kubernetes.NewForConfig(&rest.Config{Host: base, BearerToken: readonlyDevToken})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := readonly.CoreV1().Pods("default").List(c, metav1.ListOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("list pods before binding: expected Forbidden, got %v", err)
	}

	canList, err := readonly.AuthorizationV1().SelfSubjectAccessReviews().Create(c, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "list", Resource: "pods"},
		},
	}, metav1.CreateOptions{})
	if err != nil || canList.Status.Allowed {
		t.Fatalf("SelfSubjectAccessReview list pods before binding: %v %+v", err, canList.Status)
	}

	if _, err := admin.RbacV1().ClusterRoleBindings().Create(c, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "readonly-view"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "view"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "readonly"}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := readonly.CoreV1().Pods("default").List(c, metav1.ListOptions{}); err != nil {
		t.Fatalf("list pods after binding: %v", err)
	}

	if _, err := readonly.CoreV1().Pods("default").Create(c, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "denied"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
	}, metav1.CreateOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("create pod after view binding: expected Forbidden, got %v", err)
	}

	canCreate, err := readonly.AuthorizationV1().SelfSubjectAccessReviews().Create(c, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "create", Resource: "pods"},
		},
	}, metav1.CreateOptions{})
	if err != nil || canCreate.Status.Allowed {
		t.Fatalf("SelfSubjectAccessReview create pods: %v %+v", err, canCreate.Status)
	}
}

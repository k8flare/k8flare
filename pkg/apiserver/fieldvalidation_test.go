package apiserver_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestFieldValidation exercises fieldvalidation.go's `?fieldValidation=`
// support (this apiserver's substitute for real kube-apiserver's server-side
// field validation, see CLAUDE.md's known-gaps note). Uses the raw REST
// client with a hand-written JSON body (containing an unknown top-level
// field, "bogusField") rather than client-go's typed PodInterface: a typed
// *corev1.Pod has no way to carry a field the Go struct doesn't define.
func TestFieldValidation(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	_, _ = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})

	podWithUnknownField := []byte(`{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {"name": "test-field-validation-pod", "namespace": "` + ns + `"},
		"spec": {"containers": [{"name": "nginx", "image": "nginx"}]},
		"bogusField": "should not exist on a real Pod"
	}`)

	t.Run("StrictRejectsUnknownField", func(t *testing.T) {
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-pod", metav1.DeleteOptions{})

		err := client.CoreV1().RESTClient().Post().
			Resource("pods").
			Namespace(ns).
			Param("fieldValidation", "Strict").
			SetHeader("Content-Type", "application/json").
			Body(podWithUnknownField).
			Do(ctx).
			Error()
		if err == nil {
			t.Fatal("expected Strict fieldValidation to reject an unknown field, got nil error")
		}
	})

	t.Run("DefaultIsStrict", func(t *testing.T) {
		// No ?fieldValidation= at all -- matches real kube-apiserver's own
		// GA default (Strict since Kubernetes 1.27), and this project's
		// parseFieldValidation default.
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-pod", metav1.DeleteOptions{})

		err := client.CoreV1().RESTClient().Post().
			Resource("pods").
			Namespace(ns).
			SetHeader("Content-Type", "application/json").
			Body(podWithUnknownField).
			Do(ctx).
			Error()
		if err == nil {
			t.Fatal("expected default (no query param) fieldValidation to behave as Strict and reject an unknown field, got nil error")
		}
	})

	t.Run("IgnoreAcceptsUnknownField", func(t *testing.T) {
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-pod", metav1.DeleteOptions{})

		err := client.CoreV1().RESTClient().Post().
			Resource("pods").
			Namespace(ns).
			Param("fieldValidation", "Ignore").
			SetHeader("Content-Type", "application/json").
			Body(podWithUnknownField).
			Do(ctx).
			Error()
		if err != nil {
			t.Fatalf("expected Ignore fieldValidation to accept an unknown field, got: %v", err)
		}
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-pod", metav1.DeleteOptions{})
	})

	t.Run("WarnAcceptsUnknownFieldWithWarningHeader", func(t *testing.T) {
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-pod", metav1.DeleteOptions{})

		result := client.CoreV1().RESTClient().Post().
			Resource("pods").
			Namespace(ns).
			Param("fieldValidation", "Warn").
			SetHeader("Content-Type", "application/json").
			Body(podWithUnknownField).
			Do(ctx)
		if err := result.Error(); err != nil {
			t.Fatalf("expected Warn fieldValidation to accept an unknown field, got: %v", err)
		}

		if h := result.Warnings(); len(h) == 0 {
			t.Errorf("expected at least one warning for the unknown field, got none")
		}
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-pod", metav1.DeleteOptions{})
	})

	t.Run("ProtobufBodyUnaffected", func(t *testing.T) {
		// A body sent through the typed client-go interface negotiates
		// protobuf by default (see handler.go's decodeBody doc comment) --
		// this must still work with the new default-Strict behavior, since
		// DecodeStrict only ever applies to JSON bodies (isJSONBody).
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-protobuf-pod", metav1.DeleteOptions{})
		pod, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-field-validation-protobuf-pod", Namespace: ns},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("expected a normal typed-client Create (protobuf body) to keep working, got: %v", err)
		}
		if pod.Name != "test-field-validation-protobuf-pod" {
			t.Errorf("Name: got %q", pod.Name)
		}
		_ = client.CoreV1().Pods(ns).Delete(ctx, "test-field-validation-protobuf-pod", metav1.DeleteOptions{})
	})
}

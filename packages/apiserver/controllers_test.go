//go:build !js

package apiserver_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestControllers(t *testing.T) {
	base, cs := startDevURL(t)
	c, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	createNode(t, cs, c, "c1")
	replicas := int32(2)
	labels := map[string]string{"app": "web"}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "img"}}},
			},
		},
	}
	if _, err := cs.AppsV1().Deployments("default").Create(c, deployment, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(c, time.Second, 150*time.Second, true, func(ctx context.Context) (bool, error) {
		pods, err := cs.CoreV1().Pods("default").List(ctx, metav1.ListOptions{LabelSelector: "app=web"})
		if err != nil {
			return false, err
		}
		bound := 0
		for _, p := range pods.Items {
			if p.Spec.NodeName == "c1" {
				bound++
			}
		}
		return bound == 2, nil
	}); err != nil {
		rs, _ := cs.AppsV1().ReplicaSets("default").List(c, metav1.ListOptions{})
		pods, _ := cs.CoreV1().Pods("default").List(c, metav1.ListOptions{})
		t.Fatalf("deployment did not produce 2 bound pods: %v (replicasets=%d pods=%d)", err, len(rs.Items), len(pods.Items))
	}
	node, err := cs.CoreV1().Nodes().Get(c, "c1", metav1.GetOptions{})
	if err != nil || node.Spec.PodCIDR == "" {
		t.Fatalf("nodeipam did not assign a PodCIDR: %v %q", err, node.Spec.PodCIDR)
	}
	if _, err := cs.CoreV1().Namespaces().Create(c, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rootca"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(base + "/cacerts")
	if err != nil {
		t.Fatal(err)
	}
	caPEM, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err := wait.PollUntilContextTimeout(c, time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		cm, err := cs.CoreV1().ConfigMaps("rootca").Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return cm.Data["ca.crt"] == string(caPEM), nil
	}); err != nil {
		t.Fatalf("kube-root-ca.crt was not published: %v", err)
	}
}

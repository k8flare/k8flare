package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const computeClassAnnotation = "k8flare.com/compute"

func newClientset(url, token string) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(&rest.Config{
		Host:        url,
		BearerToken: token,
		Timeout:     60 * time.Second,
	})
}

func probeDeployment(name, image, compute string) *appsv1.Deployment {
	replicas := int32(1)
	grace := int64(1)
	labels := map[string]string{"app": name}
	annotations := map[string]string{}
	if compute != "" {
		annotations[computeClassAnnotation] = compute
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &grace,
					Containers: []corev1.Container{{
						Name:  "probe",
						Image: image,
					}},
				},
			},
		},
	}
}

func podSummary(pods []corev1.Pod) string {
	if len(pods) == 0 {
		return "no pods exist yet"
	}
	parts := make([]string, 0, len(pods))
	for _, p := range pods {
		reason := ""
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status != corev1.ConditionTrue {
				reason = fmt.Sprintf(" (%s: %s)", c.Reason, c.Message)
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%s%s", p.Name, p.Status.Phase, reason))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func waitForRunningPods(ctx context.Context, cs kubernetes.Interface, ns, selector string, want int, timeout, interval time.Duration) ([]string, error) {
	deadline := time.Now().Add(timeout)
	state := "no pods exist yet"
	for {
		pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			state = "list pods: " + err.Error()
		} else {
			running := []string{}
			for _, p := range pods.Items {
				if p.Status.Phase == corev1.PodRunning {
					running = append(running, p.Name)
				}
			}
			if len(running) >= want {
				sort.Strings(running)
				return running, nil
			}
			state = podSummary(pods.Items)
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("fewer than %d pods of %q reached Running within %s -- last seen: %s", want, selector, timeout, state)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func scaleDeployment(ctx context.Context, cs kubernetes.Interface, ns, name string, replicas int32) error {
	_, err := cs.AppsV1().Deployments(ns).UpdateScale(ctx, name, &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
	}, metav1.UpdateOptions{})
	return err
}

func waitForPodsGone(ctx context.Context, cs kubernetes.Interface, ns, selector string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	state := ""
	for {
		pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			state = "list pods: " + err.Error()
		} else if len(pods.Items) == 0 {
			return nil
		} else {
			state = podSummary(pods.Items)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("pods of %q still exist %s after the deployment was deleted -- last seen: %s", selector, timeout, state)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func deleteDeployment(ctx context.Context, cs kubernetes.Interface, ns, name string) error {
	err := cs.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func countNodes(ctx context.Context, cs kubernetes.Interface) ([]string, error) {
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		names = append(names, n.Name)
	}
	sort.Strings(names)
	return names, nil
}

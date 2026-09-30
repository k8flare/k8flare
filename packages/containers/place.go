package containers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
)

const (
	SchedulerName = "k8flare-containers"
	NodeName      = "cloudflare"

	failedSchedulingReason = "FailedScheduling"
	listPage               = 500
)

type Result struct {
	Bound    int `json:"bound"`
	Rejected int `json:"rejected"`
}

func Place(ctx context.Context, client kubernetes.Interface, declared []string) (*Result, error) {
	if err := EnsureNode(ctx, client); err != nil {
		return nil, err
	}
	result := &Result{}
	opts := metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.schedulerName", SchedulerName).String(), Limit: listPage}
	for {
		list, err := client.CoreV1().Pods("").List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range list.Items {
			pod := &list.Items[i]
			if pod.Spec.SchedulerName != SchedulerName || pod.Spec.NodeName != "" || pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue
			}
			annotations, err := Validate(pod, declared)
			if err != nil {
				if err := reject(ctx, client, pod, err.Error()); err != nil {
					return nil, err
				}
				result.Rejected++
				continue
			}
			if err := bind(ctx, client, pod, annotations); err != nil {
				return nil, err
			}
			result.Bound++
		}
		if list.Continue == "" {
			return result, nil
		}
		opts.Continue = list.Continue
	}
}

func bind(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod, annotations map[string]string) error {
	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace, UID: pod.UID, Annotations: annotations},
		Target:     corev1.ObjectReference{Kind: "Node", Name: NodeName},
	}
	err := client.CoreV1().Pods(pod.Namespace).Bind(ctx, binding, metav1.CreateOptions{})
	if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func reject(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod, message string) error {
	condition := &corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: message}
	if podutil.UpdatePodCondition(&pod.Status, condition) {
		if _, err := client.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			return err
		}
	}
	return recordEvent(ctx, client, pod, message)
}

func recordEvent(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod, message string) error {
	events := client.CoreV1().Events(pod.Namespace)
	selector := fields.Set{"involvedObject.uid": string(pod.UID), "reason": failedSchedulingReason}.AsSelector().String()
	existing, err := events.List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return err
	}
	now := metav1.Now()
	for i := range existing.Items {
		ev := &existing.Items[i]
		if ev.InvolvedObject.UID != pod.UID || ev.Reason != failedSchedulingReason || ev.Message != message || ev.Source.Component != SchedulerName {
			continue
		}
		ev.Count++
		ev.LastTimestamp = now
		_, err := events.Update(ctx, ev, metav1.UpdateOptions{})
		return err
	}
	_, err = events.Create(ctx, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s.%x", pod.Name, now.UnixNano()), Namespace: pod.Namespace},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", APIVersion: "v1", Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID, ResourceVersion: pod.ResourceVersion,
		},
		Reason:              failedSchedulingReason,
		Message:             message,
		Source:              corev1.EventSource{Component: SchedulerName},
		FirstTimestamp:      now,
		LastTimestamp:       now,
		Count:               1,
		Type:                corev1.EventTypeWarning,
		Action:              "Scheduling",
		ReportingController: SchedulerName,
		ReportingInstance:   SchedulerName,
	}, metav1.CreateOptions{})
	return err
}

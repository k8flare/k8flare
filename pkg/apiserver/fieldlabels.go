package apiserver

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
)

// The field selectors each kind accepts. client-go's scheme registers no
// field label conversions, so without these a selector such as the
// kubelet's spec.clusterIP!=None is rejected as unknown.
var fieldLabels = map[schema.GroupVersionKind][]string{
	corev1.SchemeGroupVersion.WithKind("Pod"):       {"spec.nodeName", "spec.restartPolicy", "spec.schedulerName", "spec.serviceAccountName", "spec.hostNetwork", "status.phase", "status.podIP", "status.nominatedNodeName"},
	corev1.SchemeGroupVersion.WithKind("Node"):      {"spec.unschedulable"},
	corev1.SchemeGroupVersion.WithKind("Service"):   {"spec.clusterIP", "spec.type"},
	corev1.SchemeGroupVersion.WithKind("Namespace"): {"status.phase"},
	corev1.SchemeGroupVersion.WithKind("Secret"):    {"type"},
	corev1.SchemeGroupVersion.WithKind("Event"):     {"involvedObject.kind", "involvedObject.namespace", "involvedObject.name", "involvedObject.uid", "involvedObject.apiVersion", "involvedObject.resourceVersion", "involvedObject.fieldPath", "reason", "reportingComponent", "source", "type"},
}

func init() {
	for gvk, labels := range fieldLabels {
		allowed := sets.New(labels...).Insert("metadata.name", "metadata.namespace")
		if err := scheme.Scheme.AddFieldLabelConversionFunc(gvk, func(label, value string) (string, string, error) {
			if allowed.Has(label) {
				return label, value, nil
			}
			return "", "", fmt.Errorf("field label not supported: %s", label)
		}); err != nil {
			panic(err)
		}
	}
}

func selectableFields(obj runtime.Object) fields.Set {
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil
	}
	f := fields.Set{"metadata.name": m.GetName(), "metadata.namespace": m.GetNamespace()}
	switch o := obj.(type) {
	case *corev1.Pod:
		f["spec.nodeName"] = o.Spec.NodeName
		f["spec.restartPolicy"] = string(o.Spec.RestartPolicy)
		f["spec.schedulerName"] = o.Spec.SchedulerName
		f["spec.serviceAccountName"] = o.Spec.ServiceAccountName
		f["spec.hostNetwork"] = strconv.FormatBool(o.Spec.HostNetwork)
		f["status.phase"] = string(o.Status.Phase)
		f["status.podIP"] = o.Status.PodIP
		f["status.nominatedNodeName"] = o.Status.NominatedNodeName
	case *corev1.Node:
		f["spec.unschedulable"] = strconv.FormatBool(o.Spec.Unschedulable)
	case *corev1.Service:
		f["spec.clusterIP"] = o.Spec.ClusterIP
		f["spec.type"] = string(o.Spec.Type)
	case *corev1.Namespace:
		f["status.phase"] = string(o.Status.Phase)
	case *corev1.Secret:
		f["type"] = string(o.Type)
	case *corev1.Event:
		f["involvedObject.kind"] = o.InvolvedObject.Kind
		f["involvedObject.namespace"] = o.InvolvedObject.Namespace
		f["involvedObject.name"] = o.InvolvedObject.Name
		f["involvedObject.uid"] = string(o.InvolvedObject.UID)
		f["involvedObject.apiVersion"] = o.InvolvedObject.APIVersion
		f["involvedObject.resourceVersion"] = o.InvolvedObject.ResourceVersion
		f["involvedObject.fieldPath"] = o.InvolvedObject.FieldPath
		f["reason"] = o.Reason
		f["reportingComponent"] = o.ReportingController
		f["source"] = o.Source.Component
		f["type"] = o.Type
	}
	return f
}

package generic

import "k8s.io/apimachinery/pkg/runtime/schema"

func AdmissionResources() []schema.GroupResource {
	return admissionResources
}

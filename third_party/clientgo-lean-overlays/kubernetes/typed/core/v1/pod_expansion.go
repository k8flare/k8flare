/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
)

// The PodExpansion interface allows manually adding extra methods to the PodInterface.
type PodExpansion interface {
	Bind(ctx context.Context, binding *v1.Binding, opts metav1.CreateOptions) error
	// Evict submits a policy/v1beta1 Eviction request to the pod's eviction subresource.
	// Equivalent to calling EvictV1beta1.
	// Deprecated: Use EvictV1() (supported in 1.22+) or EvictV1beta1().
	Evict(ctx context.Context, eviction *policyv1beta1.Eviction) error
	// EvictV1 submits a policy/v1 Eviction request to the pod's eviction subresource.
	// Supported in 1.22+.
	EvictV1(ctx context.Context, eviction *policyv1.Eviction) error
	// EvictV1beta1 submits a policy/v1beta1 Eviction request to the pod's eviction subresource.
	// Supported in 1.22+.
	EvictV1beta1(ctx context.Context, eviction *policyv1beta1.Eviction) error
	GetLogs(name string, opts *v1.PodLogOptions) *restclient.Request
	ProxyGet(scheme, name, port, path string, params map[string]string) restclient.ResponseWrapper
}

// Bind applies the provided binding to the named pod in the current namespace (binding.Namespace is ignored).

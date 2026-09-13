module github.com/k8flare/k8flare

go 1.26.7

replace (
	github.com/Microsoft/hcsshim => github.com/Microsoft/hcsshim v0.15.0-rc.1
	github.com/Mirantis/cri-dockerd => github.com/k3s-io/cri-dockerd v0.3.19-k3s5
	github.com/cilium/ebpf => github.com/cilium/ebpf v0.12.3
	github.com/cloudnativelabs/kube-router/v2 => github.com/k3s-io/kube-router/v2 v2.6.3-k3s1
	github.com/containerd/containerd/api => github.com/containerd/containerd/api v1.11.1
	github.com/containerd/containerd/v2 => github.com/k3s-io/containerd/v2 v2.3.4-k3s1.36
	github.com/containerd/imgcrypt => github.com/containerd/imgcrypt v1.1.11
	github.com/docker/distribution => github.com/docker/distribution v2.8.3+incompatible
	github.com/docker/docker => github.com/docker/docker v25.0.15-0.20260325154711-d2dbc0547253+incompatible
	github.com/emicklei/go-restful/v3 => github.com/emicklei/go-restful/v3 v3.13.0
	github.com/golang/protobuf => github.com/golang/protobuf v1.5.4
	github.com/googleapis/gax-go/v2 => github.com/googleapis/gax-go/v2 v2.12.0
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.4.2
	github.com/opencontainers/selinux => github.com/opencontainers/selinux v1.13.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/common => github.com/prometheus/common v0.67.5
	github.com/spegel-org/spegel => github.com/k3s-io/spegel v0.7.2-k3s1
	go.etcd.io/etcd/api/v3 => github.com/k3s-io/etcd/api/v3 v3.6.14-k3s1
	go.etcd.io/etcd/client/pkg/v3 => github.com/k3s-io/etcd/client/pkg/v3 v3.6.14-k3s1
	go.etcd.io/etcd/client/v3 => github.com/k3s-io/etcd/client/v3 v3.6.14-k3s1
	go.etcd.io/etcd/etcdutl/v3 => github.com/k3s-io/etcd/etcdutl/v3 v3.6.14-k3s1
	go.etcd.io/etcd/pkg/v3 => github.com/k3s-io/etcd/pkg/v3 v3.6.14-k3s1
	go.etcd.io/etcd/server/v3 => github.com/k3s-io/etcd/server/v3 v3.6.14-k3s1
	go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful => go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful v0.65.0
	golang.org/x/crypto => golang.org/x/crypto v0.47.0
	golang.org/x/net => golang.org/x/net v0.55.0
	golang.org/x/sys => golang.org/x/sys v0.40.0
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20230525234035-dd9d682886f9
	google.golang.org/grpc => google.golang.org/grpc v1.79.3
	k8s.io/api => github.com/k3s-io/kubernetes/staging/src/k8s.io/api v1.36.4-k3s1
	k8s.io/apiextensions-apiserver => github.com/k3s-io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v1.36.4-k3s1
	k8s.io/apimachinery => github.com/k3s-io/kubernetes/staging/src/k8s.io/apimachinery v1.36.4-k3s1
	k8s.io/apiserver => ./.build/apiserver-mirror
	k8s.io/cli-runtime => github.com/k3s-io/kubernetes/staging/src/k8s.io/cli-runtime v1.36.4-k3s1
	k8s.io/client-go => github.com/k3s-io/kubernetes/staging/src/k8s.io/client-go v1.36.4-k3s1
	k8s.io/cloud-provider => github.com/k3s-io/kubernetes/staging/src/k8s.io/cloud-provider v1.36.4-k3s1
	k8s.io/cluster-bootstrap => github.com/k3s-io/kubernetes/staging/src/k8s.io/cluster-bootstrap v1.36.4-k3s1
	k8s.io/code-generator => github.com/k3s-io/kubernetes/staging/src/k8s.io/code-generator v1.36.4-k3s1
	k8s.io/component-base => ./.build/component-base-mirror
	k8s.io/component-helpers => github.com/k3s-io/kubernetes/staging/src/k8s.io/component-helpers v1.36.4-k3s1
	k8s.io/controller-manager => github.com/k3s-io/kubernetes/staging/src/k8s.io/controller-manager v1.36.4-k3s1
	k8s.io/cri-api => github.com/k3s-io/kubernetes/staging/src/k8s.io/cri-api v1.36.4-k3s1
	k8s.io/cri-client => github.com/k3s-io/kubernetes/staging/src/k8s.io/cri-client v1.36.4-k3s1
	k8s.io/cri-streaming => github.com/k3s-io/kubernetes/staging/src/k8s.io/cri-streaming v1.36.4-k3s1
	k8s.io/csi-translation-lib => github.com/k3s-io/kubernetes/staging/src/k8s.io/csi-translation-lib v1.36.4-k3s1
	k8s.io/dynamic-resource-allocation => github.com/k3s-io/kubernetes/staging/src/k8s.io/dynamic-resource-allocation v1.36.4-k3s1
	k8s.io/endpointslice => github.com/k3s-io/kubernetes/staging/src/k8s.io/endpointslice v1.36.4-k3s1
	k8s.io/externaljwt => github.com/k3s-io/kubernetes/staging/src/k8s.io/externaljwt v1.36.4-k3s1
	k8s.io/klog => github.com/k3s-io/klog v1.0.0-k3s2 // k3s-release-1.x
	k8s.io/klog/v2 => github.com/k3s-io/klog/v2 v2.140.0-k3s1 // k3s-main
	k8s.io/kms => github.com/k3s-io/kubernetes/staging/src/k8s.io/kms v1.36.4-k3s1
	k8s.io/kube-aggregator => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-aggregator v1.36.4-k3s1
	k8s.io/kube-controller-manager => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-controller-manager v1.36.4-k3s1
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912
	k8s.io/kube-proxy => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-proxy v1.36.4-k3s1
	k8s.io/kube-scheduler => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-scheduler v1.36.4-k3s1
	k8s.io/kubectl => github.com/k3s-io/kubernetes/staging/src/k8s.io/kubectl v1.36.4-k3s1
	k8s.io/kubelet => github.com/k3s-io/kubernetes/staging/src/k8s.io/kubelet v1.36.4-k3s1
	k8s.io/kubernetes => github.com/k3s-io/kubernetes v1.36.4-k3s1
	k8s.io/metrics => github.com/k3s-io/kubernetes/staging/src/k8s.io/metrics v1.36.4-k3s1
	k8s.io/mount-utils => github.com/k3s-io/kubernetes/staging/src/k8s.io/mount-utils v1.36.4-k3s1
	k8s.io/node-api => github.com/k3s-io/kubernetes/staging/src/k8s.io/node-api v1.36.2-k3s1
	k8s.io/pod-security-admission => github.com/k3s-io/kubernetes/staging/src/k8s.io/pod-security-admission v1.36.4-k3s1
	k8s.io/sample-apiserver => github.com/k3s-io/kubernetes/staging/src/k8s.io/sample-apiserver v1.36.4-k3s1
	k8s.io/sample-cli-plugin => github.com/k3s-io/kubernetes/staging/src/k8s.io/sample-cli-plugin v1.36.4-k3s1
	k8s.io/sample-controller => github.com/k3s-io/kubernetes/staging/src/k8s.io/sample-controller v1.36.4-k3s1
	k8s.io/streaming => github.com/k3s-io/kubernetes/staging/src/k8s.io/streaming v1.36.4-k3s1
	sigs.k8s.io/cri-tools => github.com/k3s-io/cri-tools v1.36.0-k3s1
	sourcegraph.com/sourcegraph/go-diff => github.com/sourcegraph/go-diff v0.6.0
)

require (
	github.com/emicklei/go-restful/v3 v3.13.0
	k8s.io/api v0.36.4
	k8s.io/apimachinery v0.36.4
	k8s.io/apiserver v0.36.4
	k8s.io/client-go v0.36.4
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.3-0.20220714215716-96bad1d688c5 // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.28.1 // indirect
	github.com/onsi/gomega v1.39.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.etcd.io/etcd/client/v3 v3.6.14 // indirect
	go.etcd.io/etcd/server/v3 v3.6.14 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/component-base v0.36.4 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260319004828-5883c5ee87b9 // indirect
	k8s.io/streaming v0.36.4 // indirect
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.3 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

package main

type dynamicSource struct {
	controller string
	file       string
	function   string
}

var shardPatterns = []string{"./packages/workloads/shards/..."}

var servedRegistryFile = "packages/apiserver-registry/zz_generated_resources.go"

var outsideConstructors = map[string][]string{
	"disruption":    {"jobs"},
	"resourcequota": {"networking.k8s.io"},
	"tainteviction": {"nodes", "pods"},
	"volumeexpand":  {"persistentvolumes"},
}

var dynamicInformers = []dynamicSource{
	{controller: "resourcequota", file: "packages/workloads/quota.go", function: "quotaExample"},
}

var routedElsewhere = []string{"namespaces", "nodes", "pods", "serviceaccounts"}

var listedWithoutEventHandler = []string{"deviceclasses"}

var wakeWithheld = []string{"ipaddresses"}

var extraWorkloadPrefixes = []string{
	"/registry/certificates.k8s.io/",
	"/registry/gateway.networking.k8s.io/",
	"/registry/horizontalpodautoscalers/",
	"/registry/ingressclasses/",
	"/registry/rbac.authorization.k8s.io/",
	"/registry/storage.k8s.io/",
	"/registry/volumeattributesclasses/",
}

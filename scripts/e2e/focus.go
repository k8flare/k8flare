package main

var sets = map[string][]string{
	"required": {
		`[sig-scheduling] SchedulerPredicates [Serial] validates that NodeSelector is respected if matching`,
		`[sig-scheduling] SchedulerPredicates [Serial] validates that NodeSelector is respected if not matching`,
		`[sig-scheduling] SchedulerPredicates [Serial] validates resource limits of pods that are allowed to run`,
		`[sig-scheduling] SchedulerPredicates [Serial] validates that there is no conflict between pods with same hostPort but different hostIP and protocol`,
		`[sig-scheduling] LimitRange should list, patch and delete a LimitRange by collection`,
		`[sig-api-machinery] Watchers`,
		`[sig-api-machinery] CustomResourceDefinition resources [Privileged:ClusterAdmin]`,
		`[sig-api-machinery] Garbage collector should delete pods created by rc when not orphaning`,
		`[sig-api-machinery] Garbage collector should orphan pods created by rc if delete options say so`,
		`[sig-api-machinery] Garbage collector should delete RS created by deployment when not orphaning`,
		`[sig-api-machinery] Garbage collector should orphan RS created by deployment when deleteOptions.PropagationPolicy is Orphan`,
		`[sig-api-machinery] Garbage collector should not delete dependents that have both valid owner and owner that's waiting for dependents to be deleted`,
		`[sig-api-machinery] Garbage collector should not be blocked by dependency circle`,
	},
	"advisory": {
		`[sig-scheduling] LimitRange should create a LimitRange with defaults and ensure pod has those defaults applied.`,
		`[sig-api-machinery] Garbage collector should keep the rc around until all its pods are deleted if the deleteOptions says so`,
		`[sig-node] Pods`,
		`[sig-node] ConfigMap`,
		`[sig-node] Secrets`,
		`[sig-api-machinery] Namespaces [Serial]`,
		`[sig-apps] ReplicaSet`,
		`[sig-apps] Deployment`,
		`[sig-apps] Job`,
		`[sig-network] Services should serve a basic endpoint from pods`,
		`[sig-auth] ServiceAccounts`,
	},
}

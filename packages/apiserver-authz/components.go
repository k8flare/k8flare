package authz

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1helpers "k8s.io/kubernetes/pkg/apis/rbac/v1"
)

const (
	queuesUser = "system:k8flare:queues"
	nodesUser  = "system:k8flare:nodes"
)

var componentClusterRoles = []rbacv1.ClusterRole{
	{
		ObjectMeta: metav1.ObjectMeta{Name: queuesUser},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("post").URLs(
				"/internal/queue/plan",
				"/internal/queue/followup",
				"/internal/loadbalancer/provision",
				"/internal/gateway/provision",
				"/internal/extensions/dispatch",
				"/internal/metrics/scrape",
			).RuleOrDie(),
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{Name: nodesUser},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "list", "update").Groups("").Resources("pods").RuleOrDie(),
			rbacv1helpers.NewRule("get", "delete").Groups("").Resources("nodes").RuleOrDie(),
			rbacv1helpers.NewRule("post").URLs("/internal/nodes/vm-plan", "/internal/pods/merge-env", "/internal/r2/mint").RuleOrDie(),
		},
	},
}

var componentClusterRoleBindings = []rbacv1.ClusterRoleBinding{
	rbacv1helpers.NewClusterBinding(queuesUser).Users(queuesUser).BindingOrDie(),
	rbacv1helpers.NewClusterBinding(nodesUser).Users(nodesUser).BindingOrDie(),
}

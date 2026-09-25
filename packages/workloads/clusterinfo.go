package workloads

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
)

const clusterInfoRole = "kubeadm:bootstrap-signer-clusterinfo"

var ClusterServer = "https://api.k8flare.com"

func ensureClusterInfo(ctx context.Context, client kubernetes.Interface, rootCA []byte) {
	if len(rootCA) == 0 {
		return
	}
	_, err := client.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(ctx, bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
	if err == nil {
		return
	}
	if !apierrors.IsNotFound(err) {
		println("workloads: cluster-info get:", err.Error())
		return
	}
	kubeconfig, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"": {Server: ClusterServer, CertificateAuthorityData: rootCA},
		},
	})
	if err != nil {
		println("workloads: cluster-info kubeconfig:", err.Error())
		return
	}
	_, err = client.CoreV1().ConfigMaps(metav1.NamespacePublic).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: bootstrapapi.ConfigMapClusterInfo, Namespace: metav1.NamespacePublic},
		Data:       map[string]string{bootstrapapi.KubeConfigKey: string(kubeconfig)},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		println("workloads: cluster-info create:", err.Error())
		return
	}
	ensureClusterInfoRBAC(ctx, client)
}

func ensureClusterInfoRBAC(ctx context.Context, client kubernetes.Interface) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: clusterInfoRole, Namespace: metav1.NamespacePublic},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{"get"},
			APIGroups:     []string{""},
			Resources:     []string{"configmaps"},
			ResourceNames: []string{bootstrapapi.ConfigMapClusterInfo},
		}},
	}
	if _, err := client.RbacV1().Roles(metav1.NamespacePublic).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("workloads: cluster-info role:", err.Error())
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterInfoRole, Namespace: metav1.NamespacePublic},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: clusterInfoRole},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: user.Anonymous}},
	}
	if _, err := client.RbacV1().RoleBindings(metav1.NamespacePublic).Create(ctx, binding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("workloads: cluster-info rolebinding:", err.Error())
	}
}

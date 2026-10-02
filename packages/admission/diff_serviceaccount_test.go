package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

func diffServiceAccount(name string, mutate func(*corev1.ServiceAccount)) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
	if mutate != nil {
		mutate(sa)
	}
	return sa
}

func saPod(mutate func(*corev1.Pod)) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Spec.ServiceAccountName = "default"
		if mutate != nil {
			mutate(p)
		}
	})
}

func TestDiffServiceAccount(t *testing.T) {
	const plugin = "ServiceAccount"
	def := diffServiceAccount("default", nil)
	noMount := diffServiceAccount("default", func(s *corev1.ServiceAccount) { s.AutomountServiceAccountToken = ptr.To(false) })
	forceMount := diffServiceAccount("default", func(s *corev1.ServiceAccount) { s.AutomountServiceAccountToken = ptr.To(true) })
	pulls := diffServiceAccount("default", func(s *corev1.ServiceAccount) {
		s.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "reg"}, {Name: "reg2"}}
	})
	custom := diffServiceAccount("custom", nil)
	enforcing := diffServiceAccount("default", func(s *corev1.ServiceAccount) {
		s.Annotations = map[string]string{"kubernetes.io/enforce-mountable-secrets": "true"}
		s.Secrets = []corev1.ObjectReference{{Name: "allowed"}}
	})
	secretVolume := func(name string) corev1.Volume {
		return corev1.Volume{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name}}}
	}
	mirror := map[string]string{"kubernetes.io/config.mirror": "hash"}

	var cases []diffCase
	add := func(name string, pod *corev1.Pod, cluster ...runtime.Object) *diffCase {
		cases = append(cases, bothPhases(podDiffCase(plugin, name, pod), cluster...)...)
		return &cases[len(cases)-2]
	}
	add("default service account present", saPod(nil), def)
	add("no service account name uses default", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "" }), def)
	add("named service account present", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "custom" }), custom)
	add("named service account missing", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "custom" }), def)
	add("named service account in another namespace", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "custom" }), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "custom", Namespace: "other"}})
	add("pod opts out of the token mount", saPod(func(p *corev1.Pod) { p.Spec.AutomountServiceAccountToken = ptr.To(false) }), def)
	add("service account opts out of the token mount", saPod(nil), noMount)
	add("pod opts in over a service account opt-out", saPod(func(p *corev1.Pod) { p.Spec.AutomountServiceAccountToken = ptr.To(true) }), noMount)
	add("service account opts in explicitly", saPod(nil), forceMount)
	add("image pull secrets copied", saPod(nil), pulls)
	add("pod image pull secrets kept", saPod(func(p *corev1.Pod) { p.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "mine"}} }), pulls)
	add("init containers also mount the token", saPod(func(p *corev1.Pod) {
		p.Spec.InitContainers = []corev1.Container{{Name: "init", Image: "img"}}
	}), def)
	add("container already mounting the token path", saPod(func(p *corev1.Pod) {
		p.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "mine", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount"}}
		p.Spec.Volumes = []corev1.Volume{{Name: "mine", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	}), def)
	add("pod already has a token volume", saPod(func(p *corev1.Pod) {
		p.Spec.Volumes = []corev1.Volume{{Name: "kube-api-access-abcde", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	}), def)
	add("two containers", saPod(func(p *corev1.Pod) {
		p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: "c2", Image: "img"})
	}), def)
	add("pod with other volumes", saPod(func(p *corev1.Pod) {
		p.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	}), def)
	add("secret not mountable but enforcement annotation set", saPod(func(p *corev1.Pod) { p.Spec.Volumes = []corev1.Volume{secretVolume("other")} }), enforcing)
	add("secret mountable with enforcement annotation set", saPod(func(p *corev1.Pod) { p.Spec.Volumes = []corev1.Volume{secretVolume("allowed")} }), enforcing)
	add("secret volume without enforcement", saPod(func(p *corev1.Pod) { p.Spec.Volumes = []corev1.Volume{secretVolume("other")} }), def)
	add("mirror pod without references", saPod(func(p *corev1.Pod) { p.Annotations = mirror; p.Spec.ServiceAccountName = "" }), def)
	add("mirror pod naming a service account", saPod(func(p *corev1.Pod) { p.Annotations = mirror }), def)
	add("mirror pod referencing a secret", saPod(func(p *corev1.Pod) {
		p.Annotations = mirror
		p.Spec.ServiceAccountName = ""
		p.Spec.Volumes = []corev1.Volume{secretVolume("s")}
	}), def)
	add("mirror pod projecting a service account token", saPod(func(p *corev1.Pod) {
		p.Annotations = mirror
		p.Spec.ServiceAccountName = ""
		p.Spec.Volumes = []corev1.Volume{{Name: "t", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token"}}}}}}}
	}), def)

	missingDefault := podDiffCase(plugin, "default service account missing", saPod(nil))
	cases = append(cases, bothPhases(missingDefault)...)

	update := podDiffCase(plugin, "update is ignored", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "gone" }))
	update.operation = "UPDATE"
	update.oldObject = saPod(nil)
	cases = append(cases, bothPhases(update, def)...)

	status := podDiffCase(plugin, "status subresource is ignored", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "gone" }))
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = saPod(nil)
	cases = append(cases, bothPhases(status, def)...)

	ephemeral := func(name string, pod *corev1.Pod, sa *corev1.ServiceAccount) {
		c := podDiffCase(plugin, name, pod)
		c.operation = "UPDATE"
		c.subresource = "ephemeralcontainers"
		c.oldObject = saPod(nil)
		cases = append(cases, bothPhases(c, sa)...)
	}
	ephemeralPod := func(secret string) *corev1.Pod {
		return saPod(func(p *corev1.Pod) {
			p.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{
				Name: "dbg", Image: "img",
				Env: []corev1.EnvVar{{Name: "E", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secret}, Key: "k"}}}},
			}}}
		})
	}
	ephemeral("ephemeral container referencing a secret the service account does not list", ephemeralPod("other"), enforcing)
	ephemeral("ephemeral container referencing a listed secret", ephemeralPod("allowed"), enforcing)
	ephemeral("ephemeral container without enforcement", ephemeralPod("other"), def)

	deleted := podDiffCase(plugin, "delete is ignored", saPod(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "gone" }))
	deleted.operation = "DELETE"
	cases = append(cases, bothPhases(deleted)...)

	allowedByOurs := "outcome: upstream denied 403 Forbidden; ours allowed"
	mirrorReason := "ours does not special-case mirror pods: it neither skips the default account and token mount nor rejects service account, secret or token projection references (serviceaccount/admission.go Admit and Validate, mirror pod branches)"
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"container already mounting the token path":                                             {reason: "ours appends the token volume even when every container already mounts the token path, upstream adds it only when a container needs it (serviceaccount/admission.go mountServiceAccountToken)", rewrites: []objectRewrite{addTokenVolume}},
		"pod already has a token volume":                                                        {reason: "upstream reuses an existing volume named kube-api-access-*, ours matches the exact name kube-api-access and appends another (serviceaccount/admission.go mountServiceAccountToken)", rewrites: []objectRewrite{addTokenVolume}},
		"secret not mountable but enforcement annotation set":                                   {reason: "ours ignores the kubernetes.io/enforce-mountable-secrets annotation (serviceaccount/admission.go enforceMountableSecrets, limitSecretReferences)", signature: allowedByOurs},
		"secret not mountable but enforcement annotation set (validate)":                        {reason: "same as the admit case", signature: allowedByOurs},
		"mirror pod without references":                                                         {reason: mirrorReason, rewrites: []objectRewrite{defaultAccountAndTokenMount}},
		"mirror pod naming a service account":                                                   {reason: mirrorReason, signature: allowedByOurs},
		"mirror pod naming a service account (validate)":                                        {reason: mirrorReason, signature: allowedByOurs},
		"mirror pod referencing a secret":                                                       {reason: mirrorReason, signature: allowedByOurs},
		"mirror pod referencing a secret (validate)":                                            {reason: mirrorReason, signature: allowedByOurs},
		"mirror pod projecting a service account token":                                         {reason: mirrorReason, signature: allowedByOurs},
		"mirror pod projecting a service account token (validate)":                              {reason: mirrorReason, signature: allowedByOurs},
		"default service account missing":                                                       {reason: "upstream retries the lookup for up to about two seconds and then rejects, ours admits a pod whose default service account does not exist yet (serviceaccount/admission.go getServiceAccount, applyServiceAccount podServiceAccount)", signature: allowedByOurs},
		"default service account missing (validate)":                                            {reason: "same as the admit case", signature: allowedByOurs},
		"ephemeral container referencing a secret the service account does not list (validate)": {reason: "ours ignores the ephemeralcontainers subresource, upstream enforces the mountable secrets there and returns a plain error (serviceaccount/admission.go limitEphemeralContainerSecretReferences)", signature: "outcome: upstream denied 500 InternalError; ours allowed"},
	})
	runDiffCases(t, cases)
}

func tokenVolume() corev1.Volume {
	return corev1.Volume{Name: "kube-api-access", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
		DefaultMode: ptr.To(int32(420)),
		Sources: []corev1.VolumeProjection{
			{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token", ExpirationSeconds: ptr.To(int64(3607))}},
			{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"}, Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
			{DownwardAPI: &corev1.DownwardAPIProjection{Items: []corev1.DownwardAPIVolumeFile{{Path: "namespace", FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"}}}}},
		},
	}}}
}

func addTokenVolume(obj runtime.Object) {
	pod := obj.(*corev1.Pod)
	pod.Spec.Volumes = append(pod.Spec.Volumes, tokenVolume())
}

func defaultAccountAndTokenMount(obj runtime.Object) {
	pod := obj.(*corev1.Pod)
	pod.Spec.ServiceAccountName = "default"
	addTokenVolume(pod)
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{Name: "kube-api-access", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true})
	}
}

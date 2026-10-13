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

	runDiffCases(t, cases)
}

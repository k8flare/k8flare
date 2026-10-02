package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func psaNamespace(labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns", Labels: labels}}
}

func restrictedContainer() corev1.Container {
	return corev1.Container{
		Name: "c", Image: "img",
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			RunAsNonRoot:             ptr.To(true),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
}

func TestDiffPodSecurity(t *testing.T) {
	const plugin = "PodSecurity"
	baseline := psaNamespace(map[string]string{"pod-security.kubernetes.io/enforce": "baseline"})
	restricted := psaNamespace(map[string]string{"pod-security.kubernetes.io/enforce": "restricted"})
	restrictedOld := psaNamespace(map[string]string{"pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.20"})
	privileged := psaNamespace(map[string]string{"pod-security.kubernetes.io/enforce": "privileged"})
	warnOnly := psaNamespace(map[string]string{"pod-security.kubernetes.io/warn": "restricted", "pod-security.kubernetes.io/audit": "restricted"})
	badLevel := psaNamespace(map[string]string{"pod-security.kubernetes.io/enforce": "bogus"})
	unlabeled := psaNamespace(nil)

	mutate := func(f func(*corev1.Pod)) *corev1.Pod { return diffPod(f) }
	privilegedPod := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptr.To(true)}
	})
	hostNetwork := mutate(func(p *corev1.Pod) { p.Spec.HostNetwork = true })
	hostPID := mutate(func(p *corev1.Pod) { p.Spec.HostPID = true })
	hostPath := mutate(func(p *corev1.Pod) {
		p.Spec.Volumes = []corev1.Volume{{Name: "h", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/etc"}}}}
	})
	hostPort := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 80, HostPort: 80}}
	})
	netAdmin := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}}}
	})
	netBind := mutate(func(p *corev1.Pod) {
		c := restrictedContainer()
		c.SecurityContext.Capabilities.Add = []corev1.Capability{"NET_BIND_SERVICE"}
		p.Spec.Containers[0] = c
	})
	unsafeSysctl := mutate(func(p *corev1.Pod) {
		p.Spec.SecurityContext = &corev1.PodSecurityContext{Sysctls: []corev1.Sysctl{{Name: "kernel.shm_rmid_forced", Value: "1"}, {Name: "net.core.somaxconn", Value: "1"}}}
	})
	compliant := mutate(func(p *corev1.Pod) { p.Spec.Containers[0] = restrictedContainer() })
	root := mutate(func(p *corev1.Pod) {
		c := restrictedContainer()
		c.SecurityContext.RunAsNonRoot = ptr.To(false)
		p.Spec.Containers[0] = c
	})
	rootUser := mutate(func(p *corev1.Pod) {
		c := restrictedContainer()
		c.SecurityContext.RunAsUser = ptr.To(int64(0))
		p.Spec.Containers[0] = c
	})
	noSeccomp := mutate(func(p *corev1.Pod) {
		c := restrictedContainer()
		c.SecurityContext.SeccompProfile = nil
		p.Spec.Containers[0] = c
	})
	unconfinedSeccomp := mutate(func(p *corev1.Pod) {
		c := restrictedContainer()
		c.SecurityContext.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined}
		p.Spec.Containers[0] = c
	})
	emptyDirOnly := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0] = restrictedContainer()
		p.Spec.Volumes = []corev1.Volume{{Name: "e", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	})
	nfs := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0] = restrictedContainer()
		p.Spec.Volumes = []corev1.Volume{{Name: "n", VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: "s", Path: "/p"}}}}
	})
	initPrivileged := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0] = restrictedContainer()
		p.Spec.InitContainers = []corev1.Container{{Name: "i", Image: "img", SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)}}}
	})
	appArmor := mutate(func(p *corev1.Pod) {
		p.Annotations = map[string]string{"container.apparmor.security.beta.kubernetes.io/c": "unconfined"}
	})
	procMount := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{ProcMount: ptr.To(corev1.UnmaskedProcMount)}
	})
	selinux := mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{SELinuxOptions: &corev1.SELinuxOptions{Type: "spc_t"}}
	})
	windowsHostProcess := mutate(func(p *corev1.Pod) {
		p.Spec.SecurityContext = &corev1.PodSecurityContext{WindowsOptions: &corev1.WindowsSecurityContextOptions{HostProcess: ptr.To(true)}}
	})

	var cases []diffCase
	add := func(name string, pod *corev1.Pod, ns *corev1.Namespace) {
		c := podDiffCase(plugin, name, pod)
		c.phase = "validate"
		if ns != nil {
			c.cluster = []runtime.Object{ns}
		}
		cases = append(cases, c)
	}
	add("privileged pod in an unlabeled namespace", privilegedPod, unlabeled)
	add("privileged pod without a namespace object", privilegedPod, nil)
	add("privileged pod in a privileged namespace", privilegedPod, privileged)
	add("privileged pod in a baseline namespace", privilegedPod, baseline)
	add("host network in a baseline namespace", hostNetwork, baseline)
	add("host PID in a baseline namespace", hostPID, baseline)
	add("host path in a baseline namespace", hostPath, baseline)
	add("host port in a baseline namespace", hostPort, baseline)
	add("NET_ADMIN in a baseline namespace", netAdmin, baseline)
	add("unsafe sysctl in a baseline namespace", unsafeSysctl, baseline)
	add("AppArmor unconfined in a baseline namespace", appArmor, baseline)
	add("unmasked procMount in a baseline namespace", procMount, baseline)
	add("custom SELinux type in a baseline namespace", selinux, baseline)
	add("Windows host process in a baseline namespace", windowsHostProcess, baseline)
	add("privileged init container in a baseline namespace", initPrivileged, baseline)
	add("plain pod in a baseline namespace", diffPod(nil), baseline)
	add("plain pod in a restricted namespace", diffPod(nil), restricted)
	add("compliant pod in a restricted namespace", compliant, restricted)
	add("run as non-root false in a restricted namespace", root, restricted)
	add("run as user 0 in a restricted namespace", rootUser, restricted)
	add("missing seccomp in a restricted namespace", noSeccomp, restricted)
	add("unconfined seccomp in a restricted namespace", unconfinedSeccomp, restricted)
	add("NET_BIND_SERVICE in a restricted namespace", netBind, restricted)
	add("emptyDir in a restricted namespace", emptyDirOnly, restricted)
	add("NFS volume in a restricted namespace", nfs, restricted)
	add("plain pod against an older restricted version", diffPod(nil), restrictedOld)
	add("compliant pod against an older restricted version", compliant, restrictedOld)
	add("plain pod with only warn and audit labels", diffPod(nil), warnOnly)
	add("privileged pod with an invalid enforce level", privilegedPod, badLevel)

	updatePod := func(name string, obj, old *corev1.Pod, ns *corev1.Namespace) {
		c := podDiffCase(plugin, name, obj)
		c.phase = "validate"
		c.operation = "UPDATE"
		c.oldObject = old
		c.cluster = []runtime.Object{ns}
		cases = append(cases, c)
	}
	updatePod("update leaving a violating pod unchanged", privilegedPod, privilegedPod, baseline)
	updatePod("update making a pod violate", privilegedPod, diffPod(nil), baseline)
	updatePod("update changing only metadata on a violating pod", mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptr.To(true)}
		p.Labels = map[string]string{"a": "b"}
	}), privilegedPod, baseline)
	updatePod("update changing the image of a violating pod", mutate(func(p *corev1.Pod) {
		p.Spec.Containers[0].Image = "other"
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptr.To(true)}
	}), privilegedPod, baseline)
	updatePod("update fixing a violating pod", diffPod(nil), privilegedPod, baseline)

	status := podDiffCase(plugin, "status subresource is ignored", privilegedPod)
	status.phase = "validate"
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = diffPod(nil)
	status.cluster = []runtime.Object{baseline}
	ephemeral := podDiffCase(plugin, "ephemeralcontainers subresource is checked", mutate(func(p *corev1.Pod) {
		p.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "dbg", Image: "img", SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)}}}}
	}))
	ephemeral.phase = "validate"
	ephemeral.operation = "UPDATE"
	ephemeral.subresource = "ephemeralcontainers"
	ephemeral.oldObject = diffPod(nil)
	ephemeral.cluster = []runtime.Object{baseline}
	deleted := podDiffCase(plugin, "delete is ignored", privilegedPod)
	deleted.phase = "validate"
	deleted.operation = "DELETE"
	deleted.cluster = []runtime.Object{baseline}
	other := podDiffCase(plugin, "other resource is ignored", privilegedPod)
	other.phase = "validate"
	other.resource = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	other.cluster = []runtime.Object{baseline}
	cases = append(cases, status, ephemeral, deleted, other)
	updateReason := "upstream skips an update whose containers differ only in ways other than image (isSignificantPodUpdate), ours evaluates every update, so it rejects a metadata-only update of an already violating pod (pod-security-admission admission.go ValidatePod)"
	denied := "outcome: upstream allowed; ours denied 403 Forbidden"
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"privileged pod without a namespace object":        {reason: "upstream fails with InternalError when the namespace cannot be fetched, ours treats a missing namespace as unlabeled (pod-security-admission admission.go ValidatePod)", signature: "outcome: upstream denied 500 InternalError; ours allowed"},
		"update leaving a violating pod unchanged":         {reason: updateReason, signature: denied},
		"update making a pod violate":                      {reason: "upstream considers a security-context-only change insignificant and allows it (the pod validation rejects it elsewhere), ours evaluates and rejects (pod-security-admission admission.go isSignificantContainerUpdate)", signature: denied},
		"update changing only metadata on a violating pod": {reason: updateReason, signature: denied},
	})
	runDiffCases(t, cases)
}

package containers

import (
	"encoding/json"
	"fmt"
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	InstanceAnnotation = "containers.k8flare.com/instance"
	ImageAnnotation    = "containers.k8flare.com/image"

	DefaultInstance = "lite"

	customMinVCPU      = 1.0
	customMaxVCPU      = 4.0
	customMibPerVCPU   = 3072
	customMaxMemoryMib = 12288
	customMinDiskMb    = 2000
	customMaxDiskMb    = 20000
	mib                = 1024 * 1024
	mb                 = 1000 * 1000
)

type namedInstance struct {
	name   string
	cpu    resource.Quantity
	memory resource.Quantity
	disk   resource.Quantity
}

var namedInstances = []namedInstance{
	{name: "lite", cpu: resource.MustParse("62.5m"), memory: resource.MustParse("256Mi"), disk: resource.MustParse("2000M")},
	{name: "basic", cpu: resource.MustParse("250m"), memory: resource.MustParse("1Gi"), disk: resource.MustParse("4000M")},
	{name: "standard-1", cpu: resource.MustParse("500m"), memory: resource.MustParse("4Gi"), disk: resource.MustParse("8000M")},
}

type customInstance struct {
	VCPU      float64 `json:"vcpu"`
	MemoryMib int64   `json:"memoryMib"`
	DiskMb    int64   `json:"diskMb"`
}

func InstanceFor(pod *corev1.Pod) (string, error) {
	cpu, memory, disk := podFootprint(pod)
	if cpu.IsZero() && memory.IsZero() && disk.IsZero() {
		return DefaultInstance, nil
	}
	if cpu.MilliValue() < 1000 {
		return namedInstanceFor(cpu, memory, disk)
	}
	return customInstanceFor(cpu, memory, disk)
}

func podFootprint(pod *corev1.Pod) (cpu, memory, disk resource.Quantity) {
	for _, c := range pod.Spec.Containers {
		cpu = maxQuantity(cpu, *c.Resources.Requests.Cpu(), *c.Resources.Limits.Cpu())
		memory = maxQuantity(memory, *c.Resources.Requests.Memory(), *c.Resources.Limits.Memory())
		disk = maxQuantity(disk, *c.Resources.Requests.StorageEphemeral(), *c.Resources.Limits.StorageEphemeral())
	}
	return cpu, memory, disk
}

func maxQuantity(current resource.Quantity, candidates ...resource.Quantity) resource.Quantity {
	for _, q := range candidates {
		if q.Cmp(current) > 0 {
			current = q
		}
	}
	return current
}

func namedInstanceFor(cpu, memory, disk resource.Quantity) (string, error) {
	for _, n := range namedInstances {
		if cpu.Cmp(n.cpu) <= 0 && memory.Cmp(n.memory) <= 0 && disk.Cmp(n.disk) <= 0 {
			return n.name, nil
		}
	}
	largest := namedInstances[len(namedInstances)-1]
	return "", fmt.Errorf("pod below 1 vCPU (%s) needs %s memory and %s disk, more than the largest named instance %s (%s memory, %s disk): request at least 1 vCPU for a custom instance",
		cpu.String(), memory.String(), disk.String(), largest.name, largest.memory.String(), largest.disk.String())
}

func customInstanceFor(cpu, memory, disk resource.Quantity) (string, error) {
	vcpu := float64(cpu.MilliValue()) / 1000
	memoryMib := ceilDiv(memory.Value(), mib)
	diskMb := ceilDiv(disk.Value(), mb)
	if diskMb < customMinDiskMb {
		diskMb = customMinDiskMb
	}
	if vcpu > customMaxVCPU {
		return "", fmt.Errorf("cpu %s exceeds the custom instance maximum of %g vCPU", cpu.String(), customMaxVCPU)
	}
	if minMib := int64(math.Ceil(vcpu * customMibPerVCPU)); memoryMib < minMib {
		return "", fmt.Errorf("memory %d MiB is below the custom instance minimum of %d MiB per vCPU (%d MiB for %g vCPU)", memoryMib, customMibPerVCPU, minMib, vcpu)
	}
	if memoryMib > customMaxMemoryMib {
		return "", fmt.Errorf("memory %d MiB exceeds the custom instance maximum of %d MiB", memoryMib, customMaxMemoryMib)
	}
	if diskMb > customMaxDiskMb {
		return "", fmt.Errorf("ephemeral-storage %d MB exceeds the custom instance maximum of %d MB", diskMb, customMaxDiskMb)
	}
	out, err := json.Marshal(customInstance{VCPU: vcpu, MemoryMib: memoryMib, DiskMb: diskMb})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func ceilDiv(n, d int64) int64 {
	return (n + d - 1) / d
}

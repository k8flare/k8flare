package edgehost

import (
	"encoding/json"
	"net/http"
	"sort"
)

const nodeReadyTimeoutMs int64 = 5 * 60 * 1000

type vmTrack struct {
	Namespace       string `json:"namespace"`
	PodName         string `json:"podName"`
	PodUID          string `json:"podUID"`
	NodeName        string `json:"nodeName"`
	Tier            string `json:"tier"`
	Bound           bool   `json:"bound"`
	Started         bool   `json:"started"`
	BootedAt        int64  `json:"bootedAt"`
	MeshConnectorID string `json:"meshConnectorId,omitempty"`
}

type vmPod struct {
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		UID               string            `json:"uid"`
		DeletionTimestamp string            `json:"deletionTimestamp"`
		Annotations       map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		NodeSelector map[string]string `json:"nodeSelector"`
	} `json:"spec"`
	Status struct {
		Phase      string `json:"phase"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"status"`
}

type vmPlanIn struct {
	NowMs   int64              `json:"nowMs"`
	Tracked map[string]vmTrack `json:"tracked"`
	Pending []vmPod            `json:"pending"`
	Pods    map[string]*vmPod  `json:"pods"`
	Nodes   map[string]*vmPod  `json:"nodes"`
}

type vmBoot struct {
	UID             string `json:"uid"`
	Namespace       string `json:"namespace"`
	PodName         string `json:"podName"`
	PodUID          string `json:"podUID"`
	NodeName        string `json:"nodeName"`
	Tier            string `json:"tier"`
	Claimed         bool   `json:"claimed"`
	MeshConnectorID string `json:"meshConnectorId,omitempty"`
}

type vmPlan struct {
	Boots     []vmBoot  `json:"boots"`
	Teardowns []vmTrack `json:"teardowns"`
	MarkBound []string  `json:"markBound"`
	HasWork   bool      `json:"hasWork"`
}

func PlanVMs(in vmPlanIn) vmPlan {
	if in.Tracked == nil {
		in.Tracked = map[string]vmTrack{}
	}
	tracked := map[string]vmTrack{}
	for k, v := range in.Tracked {
		tracked[k] = v
	}
	var boots []vmBoot
	eligible := 0
	for _, pod := range in.Pending {
		if !pendingVMPod(pod) {
			continue
		}
		eligible++
		uid := podUID(pod)
		if tracked[uid].Started {
			continue
		}
		nodeName := pod.Spec.NodeSelector["kubernetes.io/hostname"]
		tier := pod.Metadata.Annotations["k8flare.com/nodevm-tier"]
		if nodeName == "" || !vmTier(tier) {
			continue
		}
		if cur, ok := tracked[uid]; ok {
			boots = append(boots, vmBoot{
				UID: uid, Namespace: cur.Namespace, PodName: cur.PodName, PodUID: cur.PodUID,
				NodeName: cur.NodeName, Tier: cur.Tier, Claimed: true, MeshConnectorID: cur.MeshConnectorID,
			})
			continue
		}
		tracked[uid] = vmTrack{
			Namespace: pod.Metadata.Namespace, PodName: pod.Metadata.Name, PodUID: uid,
			NodeName: nodeName, Tier: tier, BootedAt: in.NowMs,
		}
		boots = append(boots, vmBoot{
			UID: uid, Namespace: pod.Metadata.Namespace, PodName: pod.Metadata.Name, PodUID: uid,
			NodeName: nodeName, Tier: tier,
		})
	}
	var teardowns []vmTrack
	var markBound []string
	uids := make([]string, 0, len(tracked))
	for uid := range tracked {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	for _, uid := range uids {
		vm := tracked[uid]
		pod := lookupPod(in, vm)
		if podGone(pod, vm.PodUID) {
			teardowns = append(teardowns, vm)
			delete(tracked, uid)
			continue
		}
		if vm.Bound {
			continue
		}
		if nodeReady(in.Nodes[vm.NodeName]) {
			vm.Bound = true
			tracked[uid] = vm
			markBound = append(markBound, uid)
			continue
		}
		if in.NowMs-vm.BootedAt > nodeReadyTimeoutMs {
			teardowns = append(teardowns, vm)
			delete(tracked, uid)
		}
	}
	if boots == nil {
		boots = []vmBoot{}
	}
	if teardowns == nil {
		teardowns = []vmTrack{}
	}
	if markBound == nil {
		markBound = []string{}
	}
	return vmPlan{Boots: boots, Teardowns: teardowns, MarkBound: markBound, HasWork: eligible > 0 || len(tracked) > 0}
}

func PlanVMsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var in vmPlanIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(PlanVMs(in))
}

func pendingVMPod(pod vmPod) bool {
	if pod.Metadata.DeletionTimestamp != "" {
		return false
	}
	sel := pod.Spec.NodeSelector
	return sel["k8flare.com/backend"] == "containers" && sel["kubernetes.io/hostname"] != ""
}

func podUID(pod vmPod) string {
	if pod.Metadata.UID != "" {
		return pod.Metadata.UID
	}
	return pod.Metadata.Namespace + "/" + pod.Metadata.Name
}

func vmTier(tier string) bool {
	return tier == "small" || tier == "medium" || tier == "large"
}

func lookupPod(in vmPlanIn, vm vmTrack) *vmPod {
	if in.Pods != nil {
		if pod, ok := in.Pods[vm.Namespace+"/"+vm.PodName]; ok {
			return pod
		}
	}
	for i := range in.Pending {
		p := &in.Pending[i]
		if p.Metadata.Namespace == vm.Namespace && p.Metadata.Name == vm.PodName {
			return p
		}
	}
	return nil
}

func podGone(pod *vmPod, uid string) bool {
	if pod == nil || pod.Metadata.UID != uid || pod.Metadata.DeletionTimestamp != "" {
		return true
	}
	return pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed"
}

func nodeReady(node *vmPod) bool {
	if node == nil {
		return false
	}
	for _, c := range node.Status.Conditions {
		if c.Type == "Ready" && c.Status == "True" {
			return true
		}
	}
	return false
}

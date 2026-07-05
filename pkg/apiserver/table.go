package apiserver

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
)

// wantsTable reports whether r requests the meta.k8s.io/v1 Table
// representation kubectl's default human-readable "get" output uses, e.g.
// "Accept: application/json;as=Table;v=v1;g=meta.k8s.io". Checked with a
// substring match rather than full media-type parsing: kubectl's Accept
// header is one of a small, well-known fixed set (this parameter, or none),
// so a strict parser buys no correctness this project's real client
// (kubectl, not arbitrary user agents) would exercise, at more code.
func wantsTable(r *http.Request) bool {
	for _, accept := range r.Header.Values("Accept") {
		for _, part := range strings.Split(accept, ",") {
			if strings.Contains(part, "as=Table") {
				return true
			}
		}
	}
	return false
}

// ConvertToTable builds the meta.k8s.io/v1 Table representation of obj (a
// single resource, or a list of them) for the resource kinds kubectl's
// "kubectl get" is exercised against in this project's conformance runs
// (docs/general-purpose-k8s-plan.md): pods, nodes, deployments, replicasets,
// services, and namespaces. Anything else falls back to the NAME+AGE columns
// real kube-apiserver also falls back to for a resource with no
// additionalPrinterColumns (e.g. a CRD) -- this project has no CRD-defined
// printer columns to read, so that fallback is also this function's ceiling
// for every other built-in type. Reusing the real upstream table printers
// (k8s.io/kubernetes/pkg/printers/internalversion) was considered first per
// CLAUDE.md rule 3, but they pull in the full internal-typed conversion
// machinery those printers are generated against; given the Workers 10MiB
// WASM budget this project's other components are already tight against
// (see docs/cost-model.md), a small hand-written column set for the
// resources actually exercised is the pragmatic tradeoff, recorded here per
// CLAUDE.md rule 4.
func ConvertToTable(obj runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{TypeMeta: metav1.TypeMeta{Kind: "Table", APIVersion: "meta.k8s.io/v1"}}

	if meta.IsListType(obj) {
		if lm, ok := obj.(metav1.ListMetaAccessor); ok {
			table.ListMeta = *lm.GetListMeta().(*metav1.ListMeta)
		}

		items, err := meta.ExtractList(obj)
		if err != nil {
			return nil, fmt.Errorf("convert to table: extract list: %w", err)
		}
		table.ColumnDefinitions = columnDefinitionsFor(listItemKind(obj))
		for _, item := range items {
			row, err := tableRowFor(item)
			if err != nil {
				return nil, err
			}
			table.Rows = append(table.Rows, row)
		}
		return table, nil
	}

	table.ColumnDefinitions = columnDefinitionsFor(itemKind(obj))
	row, err := tableRowFor(obj)
	if err != nil {
		return nil, err
	}
	table.Rows = []metav1.TableRow{row}
	return table, nil
}

// itemKind returns the Kind name (e.g. "Pod") columnDefinitionsFor and
// tableCellsFor switch on, for a single (non-list) object.
func itemKind(obj runtime.Object) string {
	switch obj.(type) {
	case *corev1.Pod:
		return "Pod"
	case *corev1.Node:
		return "Node"
	case *corev1.Service:
		return "Service"
	case *corev1.Namespace:
		return "Namespace"
	case *appsv1.Deployment:
		return "Deployment"
	case *appsv1.ReplicaSet:
		return "ReplicaSet"
	default:
		return ""
	}
}

// listItemKind is itemKind's counterpart for a list object with zero items,
// where columnDefinitionsFor still needs to know what columns to advertise
// even though there's no item to type-switch on directly.
func listItemKind(obj runtime.Object) string {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	return strings.TrimSuffix(kind, "List")
}

// columnDefinitionsFor returns the column headers "kubectl get" shows by
// default for kind, matching real kube-apiserver's non-wide output. Every
// kind not explicitly listed gets the same NAME+AGE fallback real
// kube-apiserver uses for a type with no registered additionalPrinterColumns.
func columnDefinitionsFor(kind string) []metav1.TableColumnDefinition {
	name := metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"}
	age := metav1.TableColumnDefinition{Name: "Age", Type: "string"}

	switch kind {
	case "Pod":
		return []metav1.TableColumnDefinition{
			name,
			{Name: "Ready", Type: "string"},
			{Name: "Status", Type: "string"},
			{Name: "Restarts", Type: "string"},
			age,
		}
	case "Node":
		return []metav1.TableColumnDefinition{
			name,
			{Name: "Status", Type: "string"},
			{Name: "Roles", Type: "string"},
			age,
			{Name: "Version", Type: "string"},
		}
	case "Deployment":
		return []metav1.TableColumnDefinition{
			name,
			{Name: "Ready", Type: "string"},
			{Name: "Up-to-date", Type: "string"},
			{Name: "Available", Type: "string"},
			age,
		}
	case "ReplicaSet":
		return []metav1.TableColumnDefinition{
			name,
			{Name: "Desired", Type: "string"},
			{Name: "Current", Type: "string"},
			{Name: "Ready", Type: "string"},
			age,
		}
	case "Service":
		return []metav1.TableColumnDefinition{
			name,
			{Name: "Type", Type: "string"},
			{Name: "Cluster-IP", Type: "string"},
			{Name: "External-IP", Type: "string"},
			{Name: "Port(s)", Type: "string"},
			age,
		}
	case "Namespace":
		return []metav1.TableColumnDefinition{
			name,
			{Name: "Status", Type: "string"},
			age,
		}
	default:
		return []metav1.TableColumnDefinition{name, age}
	}
}

// tableRowFor builds one TableRow for a single (non-list) item. Object is
// set from Encode(item) -- reusing Encode rather than a bare json.Marshal
// means the embedded object gets the same Kind/APIVersion TypeMeta filled in
// that Encode already fills in for top-level responses (see scheme.go), not
// just bare struct fields.
func tableRowFor(item runtime.Object) (metav1.TableRow, error) {
	raw, err := Encode(item)
	if err != nil {
		return metav1.TableRow{}, fmt.Errorf("convert to table: encode row object: %w", err)
	}

	objMeta := getObjectMeta(item)
	name := ""
	if objMeta != nil {
		name = objMeta.Name
	}
	age := ageOf(item)

	cells := []interface{}{name}
	cells = append(cells, tableCellsFor(item, age)...)

	return metav1.TableRow{
		Cells:  cells,
		Object: runtime.RawExtension{Raw: raw},
	}, nil
}

// ageOf renders obj's metadata.creationTimestamp the way "kubectl get"'s AGE
// column does, via the same k8s.io/apimachinery/pkg/util/duration.HumanDuration
// upstream printers use, so e.g. "5d1h" matches real kubectl output exactly.
func ageOf(obj runtime.Object) string {
	m := getObjectMeta(obj)
	if m == nil || m.CreationTimestamp.IsZero() {
		return "<unknown>"
	}
	return duration.HumanDuration(metav1Now().Sub(m.CreationTimestamp.Time))
}

// metav1Now is a thin wrapper so ageOf reads like upstream's
// translateTimestampSince (metav1.Now().Sub(...)).
func metav1Now() metav1.Time { return metav1.Now() }

// tableCellsFor returns every cell after NAME for item, given its
// already-computed age string. item's concrete type must match the kind
// columnDefinitionsFor(itemKind(item)) was built for -- this function has no
// fallback for an unrecognized type reaching it beyond NAME+AGE, mirrored by
// the default case below.
func tableCellsFor(item runtime.Object, age string) []interface{} {
	switch o := item.(type) {
	case *corev1.Pod:
		ready, total, restarts := podReadySummary(o)
		return []interface{}{
			fmt.Sprintf("%d/%d", ready, total),
			string(o.Status.Phase),
			strconv.Itoa(restarts),
			age,
		}
	case *corev1.Node:
		return []interface{}{
			nodeStatus(o),
			nodeRoles(o),
			age,
			o.Status.NodeInfo.KubeletVersion,
		}
	case *appsv1.Deployment:
		desired := int32(1)
		if o.Spec.Replicas != nil {
			desired = *o.Spec.Replicas
		}
		return []interface{}{
			fmt.Sprintf("%d/%d", o.Status.ReadyReplicas, desired),
			strconv.Itoa(int(o.Status.UpdatedReplicas)),
			strconv.Itoa(int(o.Status.AvailableReplicas)),
			age,
		}
	case *appsv1.ReplicaSet:
		desired := int32(1)
		if o.Spec.Replicas != nil {
			desired = *o.Spec.Replicas
		}
		return []interface{}{
			strconv.Itoa(int(desired)),
			strconv.Itoa(int(o.Status.Replicas)),
			strconv.Itoa(int(o.Status.ReadyReplicas)),
			age,
		}
	case *corev1.Service:
		return []interface{}{
			serviceType(o),
			nonEmptyOr(o.Spec.ClusterIP, "<none>"),
			serviceExternalIP(o),
			servicePorts(o),
			age,
		}
	case *corev1.Namespace:
		return []interface{}{
			string(o.Status.Phase),
			age,
		}
	default:
		return []interface{}{age}
	}
}

// podReadySummary returns (ready containers, total containers, total
// restarts) the way kubectl's printers/internalversion.podColumns computes
// them (excluding init/ephemeral containers, which this project's stub Pods
// don't exercise).
func podReadySummary(pod *corev1.Pod) (ready, total, restarts int) {
	total = len(pod.Status.ContainerStatuses)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += int(cs.RestartCount)
	}
	return ready, total, restarts
}

// nodeStatus mirrors kubectl's NAME/STATUS column: "Ready"/"NotReady" from
// the Node's Ready condition, plus ",SchedulingDisabled" if cordoned.
func nodeStatus(node *corev1.Node) string {
	status := "NotReady"
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionTrue {
				status = "Ready"
			}
			break
		}
	}
	if node.Spec.Unschedulable {
		status += ",SchedulingDisabled"
	}
	return status
}

// nodeRoles mirrors kubectl's ROLES column: every
// "node-role.kubernetes.io/{role}" label's {role} suffix, sorted, joined by
// commas, or "<none>" if the node has none.
func nodeRoles(node *corev1.Node) string {
	const prefix = "node-role.kubernetes.io/"
	var roles []string
	for k := range node.Labels {
		if role := strings.TrimPrefix(k, prefix); role != k {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

func serviceType(svc *corev1.Service) string {
	if svc.Spec.Type == "" {
		return string(corev1.ServiceTypeClusterIP)
	}
	return string(svc.Spec.Type)
}

func serviceExternalIP(svc *corev1.Service) string {
	var ips []string
	ips = append(ips, svc.Spec.ExternalIPs...)
	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			ips = append(ips, ing.IP)
		}
		if ing.Hostname != "" {
			ips = append(ips, ing.Hostname)
		}
	}
	if len(ips) == 0 {
		return "<none>"
	}
	return strings.Join(ips, ",")
}

func servicePorts(svc *corev1.Service) string {
	if len(svc.Spec.Ports) == 0 {
		return "<none>"
	}
	var parts []string
	for _, p := range svc.Spec.Ports {
		s := fmt.Sprintf("%d/%s", p.Port, p.Protocol)
		if p.NodePort != 0 {
			s = fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, p.Protocol)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}

func nonEmptyOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

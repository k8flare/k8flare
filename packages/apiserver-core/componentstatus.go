package core

import (
	"context"
	"sort"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

type componentStatusREST struct {
	checks map[string]func(context.Context) error
}

var (
	_ rest.Storage              = componentStatusREST{}
	_ rest.Scoper               = componentStatusREST{}
	_ rest.Getter               = componentStatusREST{}
	_ rest.Lister               = componentStatusREST{}
	_ rest.SingularNameProvider = componentStatusREST{}
	_ rest.ShortNamesProvider   = componentStatusREST{}
)

func newComponentStatusREST(client *kine.Client) rest.Storage {
	isScheduler := func(target string) bool { return target == "scheduler" }
	isController := func(target string) bool { return target != "scheduler" }
	return componentStatusREST{checks: map[string]func(context.Context) error{
		"etcd-0": func(ctx context.Context) error {
			_, err := client.Revision(ctx)
			return err
		},
		"scheduler": func(ctx context.Context) error {
			h, err := client.Health(ctx)
			if err != nil {
				return err
			}
			return h.Controllers(isScheduler)
		},
		"controller-manager": func(ctx context.Context) error {
			h, err := client.Health(ctx)
			if err != nil {
				return err
			}
			if err := h.Queues(); err != nil {
				return err
			}
			return h.Controllers(isController)
		},
	}}
}

func (componentStatusREST) New() runtime.Object     { return &corev1.ComponentStatus{} }
func (componentStatusREST) NewList() runtime.Object { return &corev1.ComponentStatusList{} }
func (componentStatusREST) Destroy()                {}
func (componentStatusREST) NamespaceScoped() bool   { return false }
func (componentStatusREST) GetSingularName() string { return "componentstatus" }
func (componentStatusREST) ShortNames() []string    { return []string{"cs"} }
func (componentStatusREST) ConvertToTable(_ context.Context, object runtime.Object, _ runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Status", Type: "string"},
			{Name: "Message", Type: "string"},
			{Name: "Error", Type: "string"},
		},
	}
	var items []corev1.ComponentStatus
	switch o := object.(type) {
	case *corev1.ComponentStatusList:
		items = o.Items
		table.ListMeta = o.ListMeta
	case *corev1.ComponentStatus:
		items = []corev1.ComponentStatus{*o}
	default:
		return rest.NewDefaultTableConvertor(corev1.Resource("componentstatuses")).ConvertToTable(context.Background(), object, nil)
	}
	for i := range items {
		status, message, errMsg := "", "", ""
		if len(items[i].Conditions) > 0 {
			c := items[i].Conditions[0]
			status = string(c.Status)
			message = c.Message
			errMsg = c.Error
		}
		table.Rows = append(table.Rows, metav1.TableRow{
			Cells:  []interface{}{items[i].Name, status, message, errMsg},
			Object: runtime.RawExtension{Object: &items[i]},
		})
	}
	return table, nil
}

func (r componentStatusREST) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	check, ok := r.checks[name]
	if !ok {
		return nil, apierrors.NewNotFound(corev1.Resource("componentstatus"), name)
	}
	return componentStatus(ctx, name, check), nil
}

func (r componentStatusREST) List(ctx context.Context, _ *metainternalversion.ListOptions) (runtime.Object, error) {
	names := make([]string, 0, len(r.checks))
	for name := range r.checks {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]corev1.ComponentStatus, len(names))
	for i, name := range names {
		items[i] = *componentStatus(ctx, name, r.checks[name])
	}
	return &corev1.ComponentStatusList{Items: items}, nil
}

func componentStatus(ctx context.Context, name string, check func(context.Context) error) *corev1.ComponentStatus {
	condition := corev1.ComponentCondition{Type: corev1.ComponentHealthy, Status: corev1.ConditionTrue, Message: "ok"}
	if err := check(ctx); err != nil {
		condition = corev1.ComponentCondition{Type: corev1.ComponentHealthy, Status: corev1.ConditionFalse, Error: err.Error()}
	}
	return &corev1.ComponentStatus{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Conditions: []corev1.ComponentCondition{condition},
	}
}

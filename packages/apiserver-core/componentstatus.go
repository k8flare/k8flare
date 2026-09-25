package core

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

var componentStatusNames = []string{"controller-manager", "etcd-0", "scheduler"}

type componentStatusREST struct{}

var (
	_ rest.Storage              = componentStatusREST{}
	_ rest.Scoper               = componentStatusREST{}
	_ rest.Getter               = componentStatusREST{}
	_ rest.Lister               = componentStatusREST{}
	_ rest.SingularNameProvider = componentStatusREST{}
	_ rest.ShortNamesProvider   = componentStatusREST{}
)

func newComponentStatusREST() rest.Storage { return componentStatusREST{} }

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

func (componentStatusREST) Get(_ context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	for _, n := range componentStatusNames {
		if n == name {
			return healthyComponent(name), nil
		}
	}
	return nil, apierrors.NewNotFound(corev1.Resource("componentstatus"), name)
}

func (componentStatusREST) List(_ context.Context, _ *metainternalversion.ListOptions) (runtime.Object, error) {
	items := make([]corev1.ComponentStatus, len(componentStatusNames))
	names := append([]string(nil), componentStatusNames...)
	sort.Strings(names)
	for i, name := range names {
		items[i] = *healthyComponent(name)
	}
	return &corev1.ComponentStatusList{Items: items}, nil
}

func healthyComponent(name string) *corev1.ComponentStatus {
	return &corev1.ComponentStatus{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Conditions: []corev1.ComponentCondition{{
			Type:    corev1.ComponentHealthy,
			Status:  corev1.ConditionTrue,
			Message: "ok",
		}},
	}
}

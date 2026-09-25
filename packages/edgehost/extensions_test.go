package edgehost

import "testing"

func TestControllerFor(t *testing.T) {
	controllers := map[string]string{"example.com/widgets": "controller-echo"}
	if got := controllerFor("/registry/example.com/widgets/default/a", controllers); got != "controller-echo" {
		t.Fatal(got)
	}
	if got := controllerFor("/registry/other", controllers); got != "" {
		t.Fatal(got)
	}
}

package edgehost

import "strings"

const (
	MetricsServer = "metrics-server"
	ServiceLB     = "servicelb"
	EdgeRouting   = "edge-routing"
)

var disabled = map[string]bool{}

func SetDisabled(list string) {
	disabled = map[string]bool{}
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			disabled[name] = true
		}
	}
}

func Disabled(name string) bool {
	return disabled[name]
}

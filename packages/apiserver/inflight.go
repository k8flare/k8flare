package apiserver

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/server/filters"
)

const (
	DefaultMaxRequestsInflight         = 400
	DefaultMaxMutatingRequestsInflight = 200
)

var longRunningRequest = filters.BasicLongRunningRequestCheck(
	sets.NewString("watch", "proxy"),
	sets.NewString("attach", "exec", "proxy", "log", "portforward"),
)

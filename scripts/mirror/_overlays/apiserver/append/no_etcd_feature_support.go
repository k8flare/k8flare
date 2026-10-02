package feature

import "k8s.io/apiserver/pkg/storage"

type noEtcd struct{}

func (noEtcd) Supports(storage.Feature) bool { return false }

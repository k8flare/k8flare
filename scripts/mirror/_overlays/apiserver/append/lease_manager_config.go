package storagebackend

// Local copy of etcd3.LeaseManagerConfig so that this package does not link
// the etcd3 storage implementation and the etcd client, which do not build
// for GOOS=js. Added by scripts/mirror.
type LeaseManagerConfig struct {
	ReuseDurationSeconds int64
	MaxObjectCount       int64
}

func NewDefaultLeaseManagerConfig() LeaseManagerConfig {
	return LeaseManagerConfig{ReuseDurationSeconds: 60, MaxObjectCount: 1000}
}

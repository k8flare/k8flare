// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md.
package v1

import (
	rest "k8s.io/client-go/rest"
)

type BatchV1Interface interface {
	RESTClient() rest.Interface
	CronJobsGetter
	JobsGetter
}

// BatchV1Client is used to interact with features provided by the batch group.

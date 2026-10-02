package webhook

import (
	"k8s.io/client-go/rest"
)

// WebhookTransportSetup, when set, lets an embedding program replace
// net.Dial for webhook Service backends (GOOS=js cannot dial ClusterIPs).
var WebhookTransportSetup func(cfg *rest.Config, serverName string, ca []byte)

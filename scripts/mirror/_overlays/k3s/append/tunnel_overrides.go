package tunnel

import (
	"net/http"
)

// TunnelHeaderOverride lets an embedding program authenticate the
// remotedialer connect request. k8flare's control plane never sees the
// agent's TLS client certificate, so packages/agent sends the node's
// bearer token here instead. Added by scripts/mirror.
var TunnelHeaderOverride func() http.Header
var TunnelIgnoreEndpointSlices bool

func tunnelHeaders() http.Header {
	if TunnelHeaderOverride != nil {
		return TunnelHeaderOverride()
	}
	return nil
}

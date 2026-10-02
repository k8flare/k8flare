package deps

// KubeConfigOverride lets an embedding program write the agent's
// kubeconfigs itself. k8flare's control plane sits behind a TLS terminator
// that never sees client certificates, so packages/agent writes bearer-token
// kubeconfigs instead of the certificate ones above. Added by scripts/mirror.
var KubeConfigOverride func(dest, url, caCert, clientCert, clientKey string) (handled bool, err error)

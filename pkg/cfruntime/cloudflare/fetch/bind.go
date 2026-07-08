package fetch

import (
	"errors"
	"net/http"
	"syscall/js"

	"github.com/k8flare/k8flare/pkg/cfruntime/internal/jshttp"
	"github.com/k8flare/k8flare/pkg/cfruntime/internal/jsutil"
)

// fetch is a function that reproduces cloudflare fetch.
// Docs: https://developers.cloudflare.com/workers/runtime-apis/fetch/
func fetch(namespace js.Value, req *http.Request, init *RequestInit) (*http.Response, error) {
	if namespace.IsUndefined() {
		return nil, errors.New("fetch function not found")
	}
	promise := namespace.Call("fetch",
		// The Request object to fetch.
		// Docs: https://developers.cloudflare.com/workers/runtime-apis/request
		jshttp.ToJSRequest(req),
		// The content of the request.
		// Docs: https://developers.cloudflare.com/workers/runtime-apis/request#requestinit
		init.ToJS(),
	)

	jsRes, err := jsutil.AwaitPromise(promise)
	if err != nil {
		return nil, err
	}

	return jshttp.ToResponse(jsRes)
}

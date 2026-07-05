package apiserver

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
)

// parseFieldValidation extracts and validates the `?fieldValidation=` query
// parameter real kube-apiserver accepts on Create/Update (Strict, Warn, or
// Ignore -- see DecodeStrict/decodeBodyWithFieldValidation below). Defaults
// to Strict, matching real kube-apiserver's own GA default since Kubernetes
// 1.27 (KEP-2113) -- which also happens to be what every kubectl version
// since then sends explicitly unless the user passes
// --validate=false/--validate=warn, so this default is rarely even
// observable in practice.
func parseFieldValidation(r *http.Request) (string, error) {
	v := r.URL.Query().Get("fieldValidation")
	if v == "" {
		v = "Strict"
	}
	switch v {
	case "Strict", "Warn", "Ignore":
		return v, nil
	default:
		return "", fmt.Errorf("invalid fieldValidation value %q: must be one of Strict, Warn, Ignore", v)
	}
}

// isJSONBody reports whether body is JSON, as opposed to protobuf --
// client-go's other negotiated content type for Create/Update bodies (see
// handler.go's decodeBody doc comment). Only a JSON body can be checked by
// DecodeStrict: Codecs's protobuf serializer has no "unknown field"
// tracking to report through in the first place, so a protobuf body always
// falls back to plain decodeBody below regardless of fieldValidation. Every
// real Kubernetes JSON object/list starts with '{' once whitespace is
// trimmed; protobuf's wire format's first byte is a field tag and, for
// every message shape this project's protobuf serializer decodes, is never
// '{' (0x7b) in practice.
func isJSONBody(body []byte) bool {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// decodeBodyWithFieldValidation decodes body per fieldValidation's
// Strict/Warn/Ignore semantics (real kube-apiserver's own
// `?fieldValidation=` behavior, https://kubernetes.io/docs/reference/using-api/server-side-field-validation/):
//   - Ignore, or a non-JSON (protobuf) body: identical to decodeBody --
//     existing lenient behavior, no strict checking attempted.
//   - Strict: any unknown or duplicate field is a decode error.
//   - Warn: unknown/duplicate fields are returned as warnings, but the
//     object still decodes and is accepted -- caller is expected to surface
//     them as response Warning headers (see writeFieldValidationWarnings).
func decodeBodyWithFieldValidation(body []byte, fieldValidation string) (obj runtime.Object, warnings []error, err error) {
	if fieldValidation == "Ignore" || !isJSONBody(body) {
		obj, err = decodeBody(body)
		return obj, nil, err
	}

	obj, strictErrs, err := DecodeStrict(body, nil)
	if err != nil {
		return nil, nil, err
	}
	if len(strictErrs) == 0 {
		return obj, nil, nil
	}
	if fieldValidation == "Strict" {
		msgs := make([]string, len(strictErrs))
		for i, e := range strictErrs {
			msgs[i] = e.Error()
		}
		return nil, nil, fmt.Errorf("strict decoding error: %s", strings.Join(msgs, ", "))
	}
	return obj, strictErrs, nil
}

// writeFieldValidationWarnings adds one RFC 7234-style Warning response
// header per warning (real kube-apiserver's `fieldValidation=Warn` response
// shape, which kubectl already knows how to surface as "Warning:" lines).
// Must be called before the response's status line is written
// (w.WriteHeader/writeRuntimeObject), same as any other response header.
func writeFieldValidationWarnings(w http.ResponseWriter, warnings []error) {
	for _, warnErr := range warnings {
		w.Header().Add("Warning", fmt.Sprintf("299 - %q", warnErr.Error()))
	}
}

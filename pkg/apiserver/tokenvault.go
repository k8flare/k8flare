package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
)

// DecodeVaultTokens parses a kine GET /key/ca/cluster-tokens response
// body (kv.value base64-encoded JSON,
// {"tokens":[{"secret":"...","role":"..."}]}) into the TokensFunc
// entries for a provisioned (non-default) cluster's currently valid
// tokens. Shape owned by
// packages/k8flare-worker/src/clusters/tokens.ts. This package's own
// cmd/apiserver-wasm/main.go
// makes the actual HTTP round-trip (only it can reach the STORAGE
// binding) and passes the response body here for the host-testable
// parsing.
//
// A token with no role is an administrator, so a vault written before
// roles existed decodes to exactly the plain secrets it always did.
func DecodeVaultTokens(body io.Reader) ([]string, error) {
	var resp struct {
		KV *struct {
			Value string `json:"value"`
		} `json:"kv"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}
	if resp.KV == nil {
		return []string{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(resp.KV.Value)
	if err != nil {
		return nil, err
	}
	var vault struct {
		Tokens []struct {
			Secret string `json:"secret"`
			Role   string `json:"role"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &vault); err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(vault.Tokens))
	for _, t := range vault.Tokens {
		role := TokenRole(t.Role)
		if t.Role == "" || role == RoleAdmin {
			entries = append(entries, t.Secret)
			continue
		}
		// Unrecognized roles are carried, not normalized: AuthMiddleware
		// is where an unknown role fails closed, and flattening it here
		// would hand it the administrator identity instead.
		entries = append(entries, TokenEntry(role, t.Secret))
	}
	return entries, nil
}

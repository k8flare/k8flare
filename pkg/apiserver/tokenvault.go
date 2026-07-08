package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
)

// DecodeVaultTokens parses a kine GET /key/ca/cluster-tokens response
// body (kv.value base64-encoded JSON, {"tokens":[{"secret":"..."}]})
// into the list of currently valid cluster-token secrets for a
// provisioned (non-default) cluster. Shape owned by
// workers/k8flare/src/clusters/tokens.ts. This package's own
// cmd/apiserver-wasm/main.go
// makes the actual HTTP round-trip (only it can reach the STORAGE
// binding) and passes the response body here for the host-testable
// parsing.
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
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &vault); err != nil {
		return nil, err
	}
	secrets := make([]string, 0, len(vault.Tokens))
	for _, t := range vault.Tokens {
		secrets = append(secrets, t.Secret)
	}
	return secrets, nil
}

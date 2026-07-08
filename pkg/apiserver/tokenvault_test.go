package apiserver

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeVaultTokens(t *testing.T) {
	vaultJSON := `{"tokens":[{"secret":"tok-a"},{"secret":"tok-b"}]}`
	value := base64.StdEncoding.EncodeToString([]byte(vaultJSON))
	body := `{"kv":{"value":"` + value + `"}}`

	tokens, err := DecodeVaultTokens(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeVaultTokens: %v", err)
	}
	want := []string{"tok-a", "tok-b"}
	if len(tokens) != len(want) {
		t.Fatalf("got %v, want %v", tokens, want)
	}
	for i, tok := range want {
		if tokens[i] != tok {
			t.Fatalf("got %v, want %v", tokens, want)
		}
	}
}

func TestDecodeVaultTokens_NoKV(t *testing.T) {
	tokens, err := DecodeVaultTokens(strings.NewReader(`{"kv":null}`))
	if err != nil {
		t.Fatalf("DecodeVaultTokens: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("got %v, want empty", tokens)
	}
}

func TestDecodeVaultTokens_BadBase64(t *testing.T) {
	body := `{"kv":{"value":"not-valid-base64!!"}}`
	if _, err := DecodeVaultTokens(strings.NewReader(body)); err == nil {
		t.Fatal("expected an error for invalid base64, got nil")
	}
}

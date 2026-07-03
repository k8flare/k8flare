package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSignR2JWT_MatchesReferenceVector cross-checks signR2JWT against an
// independent reference implementation (Node.js, built-in `crypto`/`Buffer`
// only -- no `jose` dependency, so it doesn't just re-exercise the same
// library Cloudflare's own worked example uses) written from the same raw
// documentation this file's package doc comment cites, for a fixed input
// vector. If this test ever needs to change, regenerate the expected values
// with the Node script kept in this repo's development history rather than
// hand-editing them -- the whole point of this test is that the expected
// values come from a second, independent implementation, not from reading
// signR2JWT's own output back.
func TestSignR2JWT_MatchesReferenceVector(t *testing.T) {
	cfg := R2Config{
		AccountID:       "acct0000000000000000000000000000",
		AccessKeyID:     "keyid00000000000000000000000000",
		SecretAccessKey: "test-parent-secret-access-key",
		Bucket:          "k8flare-test-bucket",
	}
	now := time.Unix(1750000000, 0).UTC()
	ttl := 3600 * time.Second
	prefix := "pvc-test-uid-1234/"

	const wantJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhY2N0MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMCIsImlzcyI6ImtleWlkMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAiLCJhdWQiOiJhY2N0MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMC5yMi5jbG91ZGZsYXJlc3RvcmFnZS5jb20iLCJpYXQiOjE3NTAwMDAwMDAsImV4cCI6MTc1MDAwMzYwMCwiYnVja2V0IjoiazhmbGFyZS10ZXN0LWJ1Y2tldCIsInNjb3BlIjoib2JqZWN0LXJlYWQtd3JpdGUiLCJwYXRocyI6eyJwcmVmaXhQYXRocyI6WyJwdmMtdGVzdC11aWQtMTIzNC8iXSwib2JqZWN0UGF0aHMiOltdfX0.KFiPwqJlJ8D7W2U7zZ3SyIMEvwY6sLJ7OpYpkdTOEI4"
	const wantSecretAccessKey = "b805fa97735b754c704e9a16de9de2249005e7a39cdc321c0aa6423ea255e496"
	const wantSessionToken = "and0L2V5SmhiR2NpT2lKSVV6STFOaUlzSW5SNWNDSTZJa3BYVkNKOS5leUp6ZFdJaU9pSmhZMk4wTURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREF3TUNJc0ltbHpjeUk2SW10bGVXbGtNREF3TURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQWlMQ0poZFdRaU9pSmhZMk4wTURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREF3TUM1eU1pNWpiRzkxWkdac1lYSmxjM1J2Y21GblpTNWpiMjBpTENKcFlYUWlPakUzTlRBd01EQXdNREFzSW1WNGNDSTZNVGMxTURBd016WXdNQ3dpWW5WamEyVjBJam9pYXpobWJHRnlaUzEwWlhOMExXSjFZMnRsZENJc0luTmpiM0JsSWpvaWIySnFaV04wTFhKbFlXUXRkM0pwZEdVaUxDSndZWFJvY3lJNmV5SndjbVZtYVhoUVlYUm9jeUk2V3lKd2RtTXRkR1Z6ZEMxMWFXUXRNVEl6TkM4aVhTd2liMkpxWldOMFVHRjBhSE1pT2x0ZGZYMC5LRmlQd3FKbEo4RDdXMlU3elozU3lJTUV2d1k2c0xKN09wWXBrZFRPRUk0"

	jwt, err := signR2JWT(cfg, "object-read-write", prefix, ttl, now)
	if err != nil {
		t.Fatalf("signR2JWT: %v", err)
	}
	if jwt != wantJWT {
		t.Fatalf("jwt mismatch:\n got: %s\nwant: %s", jwt, wantJWT)
	}

	cred, err := mintCredentialAt(cfg, prefix, ttl, now)
	if err != nil {
		t.Fatalf("mintCredentialAt: %v", err)
	}
	if cred.SecretAccessKey != wantSecretAccessKey {
		t.Errorf("secretAccessKey = %s, want %s", cred.SecretAccessKey, wantSecretAccessKey)
	}
	if cred.SessionToken != wantSessionToken {
		t.Errorf("sessionToken = %s, want %s", cred.SessionToken, wantSessionToken)
	}
	if cred.AccessKeyID != cfg.AccessKeyID {
		t.Errorf("accessKeyId = %s, want parent's %s (must be reused unchanged)", cred.AccessKeyID, cfg.AccessKeyID)
	}
	wantExpiresAt := now.Add(ttl)
	if !cred.ExpiresAt.Equal(wantExpiresAt) {
		t.Errorf("expiresAt = %s, want %s", cred.ExpiresAt, wantExpiresAt)
	}
}

// TestSignR2JWT_NoPrefixOmitsPaths asserts a whole-bucket mint (prefix == "")
// never emits a "paths" claim at all, rather than an empty-but-present one --
// R2's docs describe omitting the field entirely as how you grant
// whole-bucket access ("Omit these fields to grant access to the entire
// bucket"), which is a meaningfully different request body than a present
// empty paths object (untested against a real account either way -- see
// spikes/s6-r2/RESEARCH.md's residual items -- so matching the documented
// shape exactly, rather than a plausible-looking alternative, matters more
// than usual here).
func TestSignR2JWT_NoPrefixOmitsPaths(t *testing.T) {
	cfg := R2Config{AccountID: "acct", AccessKeyID: "kid", SecretAccessKey: "secret", Bucket: "bucket"}
	jwt, err := signR2JWT(cfg, "object-read-only", "", time.Hour, time.Now())
	if err != nil {
		t.Fatalf("signR2JWT: %v", err)
	}
	payload := decodeJWTPayloadForTest(t, jwt)
	if _, ok := payload["paths"]; ok {
		t.Errorf("payload contains %q claim for a whole-bucket (no prefix) mint, want it entirely absent: %v", "paths", payload)
	}
}

// TestSignR2JWT_DifferentPrefixesProduceDifferentSignatures is a minimal
// sanity check that two credentials scoped to different prefixes are not
// interchangeable (different signature -- an attacker can't just edit the
// prefix in a decoded JWT and re-use someone else's signature, since R2
// verifies the HMAC over the whole payload including "paths").
func TestSignR2JWT_DifferentPrefixesProduceDifferentSignatures(t *testing.T) {
	cfg := R2Config{AccountID: "acct", AccessKeyID: "kid", SecretAccessKey: "secret", Bucket: "bucket"}
	now := time.Now()
	jwtA, err := signR2JWT(cfg, "object-read-write", "pvc-aaaa/", time.Hour, now)
	if err != nil {
		t.Fatalf("signR2JWT A: %v", err)
	}
	jwtB, err := signR2JWT(cfg, "object-read-write", "pvc-bbbb/", time.Hour, now)
	if err != nil {
		t.Fatalf("signR2JWT B: %v", err)
	}
	if jwtA == jwtB {
		t.Fatalf("two different prefixes signed identical JWTs")
	}
}

// decodeJWTPayloadForTest decodes jwt's middle (payload) segment into a
// generic map, for tests that assert on claim presence/absence rather than
// exact serialized bytes.
func decodeJWTPayloadForTest(t *testing.T, jwt string) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q does not have 3 dot-separated segments", jwt)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode jwt payload segment: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal jwt payload: %v", err)
	}
	return m
}

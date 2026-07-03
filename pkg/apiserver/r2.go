package apiserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// This file implements Cloudflare R2's local ("client-side") signing scheme
// for S3 Temporary Access Credentials -- the mechanism Phase 8's spike
// (spikes/s6-r2/RESEARCH.md) identified as the right fit for per-PVC access
// isolation: mint a credential scoped to a single key prefix, entirely
// locally (no Cloudflare API call, no shared rate-limit budget consumed),
// from a single durable parent R2 API token this cluster/tenant already
// holds. See pvcbind.go for how a prefix gets assigned to a PVC in the
// first place, and R2Config below for where the parent token's own
// Access/Secret key pair comes from.
//
// The exact JWT shape (claim names, header, HMAC key material, and the
// secretAccessKey/sessionToken derivation) is not this project's invention:
// it is copied field-for-field from Cloudflare's own documentation, fetched
// raw (not summarized) on 2026-07-03 from
// https://developers.cloudflare.com/r2/api/s3/temporary-credentials/#locally-client-side-signing
// and the worked TypeScript example at
// https://developers.cloudflare.com/r2/examples/authenticate-r2-temp-credentials/
// (both quoted in full in spikes/s6-r2/RESEARCH.md's follow-up research).
// Cross-checked during development against an independent Node.js
// (built-in crypto only, no `jose` dependency) reference implementation for
// a fixed input vector -- see r2_test.go's
// TestSignR2JWT_MatchesReferenceVector, which hardcodes that vector's
// expected output as a permanent regression check.

// R2Config holds this cluster's shared R2 bucket and the parent R2 API
// token's S3-style credential pair (Access Key ID / Secret Access Key --
// "Client ID"/"Client Secret" in the R2 dashboard). Every PersistentVolume
// bound to the "r2" StorageClass draws from this one bucket, isolated from
// every other PV only by key prefix -- see pvcbind.go and
// spikes/s6-r2/RESEARCH.md's "bucket-per-PVC vs. prefix-in-shared-bucket"
// section for why a shared bucket (not one bucket per PVC) was chosen:
// bucket-per-PVC spends down the account-wide bucket cap and the shared
// 1,200 req/5min R2 REST budget as PVC count grows, for no isolation
// benefit a prefix-scoped credential doesn't already provide.
//
// SecretAccessKey is the one durable secret this whole mechanism depends
// on: never sent to a client, never embedded in a minted credential (only
// used as an HMAC key -- see signR2JWT), and loaded from a Workers secret
// binding (R2_SECRET_ACCESS_KEY; see workers/apiserver/main.go), the same
// way K3S_TOKEN is loaded elsewhere in this package.
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
}

// currentR2Config, set once via SetR2ConfigFunc, is how pvcbind.go's
// bindPersistentVolumeClaim (called transitively from handler.go's generic
// per-resource POST dispatch, several call frames away from main.go) gets
// at this cluster's R2Config without pkg/apiserver importing
// "github.com/syumai/workers/cloudflare" itself -- that import would break
// `go test ./pkg/apiserver/...`, which runs as a normal host binary, not a
// GOOS=js/wasm one. Defaults to a zero R2Config so every existing test in
// this package (none of which call SetR2ConfigFunc) gets harmless empty
// strings rather than a nil-pointer panic.
//
// This is the same "resolve Workers environment bindings during request
// handling, not at Go init/main time" constraint workers/apiserver/main.go's
// getToken() already works around for K3S_TOKEN (env bindings aren't
// reachable before the first request reaches the WASM instance) --
// generalized to a settable func-var since, unlike getToken (owned directly
// by main.go and passed as a plain parameter to the handlers that need it),
// R2Config is needed from inside ApplyPostCreateEffects's generic dispatch,
// too many call frames removed from main.go to thread a new parameter
// through without widening handler.go's shared HandleResource signature for
// every resource type, not just PersistentVolumeClaim.
//
// No "is R2 actually configured" boolean, deliberately: main.go's
// getR2Config falls back to fixed dev placeholder values for any unset
// field, the same unconditional-fallback shape getToken() already uses for
// K3S_TOKEN ("k8flare-dev-token") -- see CLAUDE.md's local-dev-pitfalls
// list, which explicitly documents that fallback as intentional, not a bug
// to fix. A cluster that never configures real R2 secrets still binds PVCs
// and mints syntactically valid credentials; they simply fail loudly (401/
// 403 from R2 on first real use) rather than the PVC staying Pending --
// consistent with this project's existing risk tolerance for this exact
// class of dev-convenience fallback.
var currentR2Config func() R2Config = func() R2Config { return R2Config{} }

// SetR2ConfigFunc installs fn as the source of truth for currentR2Config().
// Called exactly once, from workers/apiserver/main.go's main(), before
// workers.Serve(mux).
func SetR2ConfigFunc(fn func() R2Config) {
	currentR2Config = fn
}

// Endpoint returns this account's R2 S3-compatible endpoint URL, in the
// form every S3 client (aws4fetch, boto3, aws-sdk-*, s3fs/tigrisfs FUSE
// adapters) expects as its `endpoint`/`endpoint_url` configuration.
func (c R2Config) Endpoint() string {
	return "https://" + c.AccountID + ".r2.cloudflarestorage.com"
}

// audience returns the JWT "aud" claim: the endpoint's bare host, matching
// `new URL(endpoint).host` in Cloudflare's own local-signing example
// exactly (not the full https:// URL).
func (c R2Config) audience() string {
	return c.AccountID + ".r2.cloudflarestorage.com"
}

// DefaultCredentialTTL is this project's default Temporary Access
// Credential lifetime for a freshly (re)started Pod container -- unchanged
// from Cloudflare's own local-signing helper's default
// (`ttlSeconds ?? 3600`), which is itself a reasoned middle ground between
// their "use short TTLs" security guidance and a duration long enough that
// most container (re)starts naturally re-mint before it matters. See
// docs/cost-model.md's R2 section and pvcbind.go's package doc comment for
// how this interacts with the long-running-Pod credential-refresh design.
const DefaultCredentialTTL = 1 * time.Hour

// R2Credential is a minted Temporary Access Credential together with the
// bucket/endpoint/prefix a caller needs to actually use it -- the full
// payload pvcbind.go's /internal/mint-r2-credentials handler returns to
// workers/nodes, and that workers/nodes in turn injects into a Pod's
// container as environment variables (see workers/nodes/src/virtualnode.ts).
type R2Credential struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey string    `json:"secretAccessKey"`
	SessionToken    string    `json:"sessionToken"`
	Bucket          string    `json:"bucket"`
	Endpoint        string    `json:"endpoint"`
	Prefix          string    `json:"prefix"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// r2JWTHeader is the JWT protected header Cloudflare's local-signing scheme
// expects: exactly {"alg":"HS256","typ":"JWT"}, no more and no fewer
// claims -- verified against the raw worked example, not inferred.
type r2JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// r2JWTPaths restricts a credential to specific prefixes/objects within the
// bucket. Field names (prefixPaths/objectPaths) are Cloudflare's local-JWT
// claim names specifically -- NOT the same as the Temporary Credentials
// REST API's top-level "prefixes"/"objects" request fields. That naming
// mismatch between Cloudflare's own two mechanisms (REST API vs. local
// signing) is confirmed against their raw docs, not a choice made in this
// project.
type r2JWTPaths struct {
	PrefixPaths []string `json:"prefixPaths"`
	ObjectPaths []string `json:"objectPaths"`
}

// r2JWTClaims is this scheme's JWT payload. Field declaration order here
// matches Cloudflare's own jose-based worked example byte-for-byte --
// verified against an independent Node reference implementation during
// development (r2_test.go). That exact match isn't required for
// correctness (R2 parses the payload as JSON, not as an opaque byte
// string -- only the signature is byte-sensitive, and that is computed
// over whatever bytes this process itself produces and later reproduces
// identically), but matching it removes a variable when comparing this
// implementation against Cloudflare's documentation.
type r2JWTClaims struct {
	Sub    string      `json:"sub"`
	Iss    string      `json:"iss"`
	Aud    string      `json:"aud"`
	Iat    int64       `json:"iat"`
	Exp    int64       `json:"exp"`
	Bucket string      `json:"bucket"`
	Scope  string      `json:"scope"`
	Paths  *r2JWTPaths `json:"paths,omitempty"`
}

func base64URLNoPad(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// signR2JWT builds and HS256-signs the JWT Cloudflare's local
// temporary-credential scheme requires, returning its compact
// (header.payload.signature) serialization. now is threaded in as a
// parameter (rather than calling time.Now() internally) so tests can
// assert against a fixed reference vector; MintCredential is the only
// production caller, and always passes time.Now().
func signR2JWT(cfg R2Config, scope, prefix string, ttl time.Duration, now time.Time) (string, error) {
	headerJSON, err := json.Marshal(r2JWTHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("sign r2 jwt: encode header: %w", err)
	}

	claims := r2JWTClaims{
		Sub:    cfg.AccountID,
		Iss:    cfg.AccessKeyID,
		Aud:    cfg.audience(),
		Iat:    now.Unix(),
		Exp:    now.Add(ttl).Unix(),
		Bucket: cfg.Bucket,
		Scope:  scope,
	}
	if prefix != "" {
		claims.Paths = &r2JWTPaths{PrefixPaths: []string{prefix}, ObjectPaths: []string{}}
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("sign r2 jwt: encode claims: %w", err)
	}

	signingInput := base64URLNoPad(headerJSON) + "." + base64URLNoPad(claimsJSON)

	mac := hmac.New(sha256.New, []byte(cfg.SecretAccessKey))
	mac.Write([]byte(signingInput)) // hash.Hash.Write never returns an error
	sig := mac.Sum(nil)

	return signingInput + "." + base64URLNoPad(sig), nil
}

// MintCredential mints a Temporary Access Credential scoped to prefix
// within cfg.Bucket, with "object-read-write" scope (read+write+list --
// list is required for a generic S3 client or FUSE adapter to enumerate
// its own prefix, see spikes/s6-r2/RESEARCH.md's FUSE caveat #1) and ttl
// lifetime. prefix == "" mints a whole-bucket credential (not used by
// pvcbind.go today, but kept as a real capability rather than a
// prefix-shaped-only API).
//
// This is local signing only -- no Cloudflare API call, so no added
// latency on the PVC-bind/Pod-start path and no competition with other
// control-plane traffic for R2's shared 1,200 req/5min account-wide REST
// budget (see spikes/s6-r2/RESEARCH.md and docs/cost-model.md).
func MintCredential(cfg R2Config, prefix string, ttl time.Duration) (*R2Credential, error) {
	return mintCredentialAt(cfg, prefix, ttl, time.Now())
}

// mintCredentialAt is MintCredential with an injectable clock, so
// r2_test.go can assert byte-exact output against a fixed reference vector
// instead of only "doesn't error." Not exported: no production caller
// needs a clock override.
func mintCredentialAt(cfg R2Config, prefix string, ttl time.Duration, now time.Time) (*R2Credential, error) {
	jwt, err := signR2JWT(cfg, "object-read-write", prefix, ttl, now)
	if err != nil {
		return nil, err
	}

	// Per Cloudflare's documented derivation: the temporary secret access
	// key is the lowercase-hex SHA-256 digest of the signed JWT string
	// itself (not of the signing key, not of the claims alone).
	digest := sha256.Sum256([]byte(jwt))
	secretAccessKey := hex.EncodeToString(digest[:])
	// The session token is base64("jwt/" + <signed JWT>) -- standard
	// (padded) base64, matching JavaScript's btoa, not base64url.
	sessionToken := base64.StdEncoding.EncodeToString([]byte("jwt/" + jwt))

	return &R2Credential{
		AccessKeyID:     cfg.AccessKeyID, // parent's Access Key ID, reused unchanged -- see this file's package doc comment
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
		Bucket:          cfg.Bucket,
		Endpoint:        cfg.Endpoint(),
		Prefix:          prefix,
		ExpiresAt:       now.Add(ttl),
	}, nil
}

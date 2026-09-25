package edgehost

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type R2Creds struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
}

func MintR2(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		AccountID       string `json:"accountId"`
		Bucket          string `json:"bucket"`
		Prefix          string `json:"prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	creds, err := MintLocal(in.AccessKeyID, in.SecretAccessKey, in.AccountID, in.Bucket, in.Prefix, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(creds)
}

func MintLocal(accessKey, secret, account, bucket, prefix string, now time.Time) (R2Creds, error) {
	if accessKey == "" || secret == "" || account == "" || bucket == "" || prefix == "" {
		return R2Creds{}, fmt.Errorf("r2 mint is incomplete")
	}
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(struct {
		Bucket string `json:"bucket"`
		Scope  string `json:"scope"`
		Paths  struct {
			PrefixPaths []string `json:"prefixPaths"`
			ObjectPaths []string `json:"objectPaths"`
		} `json:"paths"`
		Sub string `json:"sub"`
		Iss string `json:"iss"`
		Aud string `json:"aud"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}{
		Bucket: bucket,
		Scope:  "object-read-write",
		Paths: struct {
			PrefixPaths []string `json:"prefixPaths"`
			ObjectPaths []string `json:"objectPaths"`
		}{PrefixPaths: []string{prefix}, ObjectPaths: []string{}},
		Sub: account,
		Iss: accessKey,
		Aud: account + ".r2.cloudflarestorage.com",
		Iat: now.Unix(),
		Exp: now.Unix() + 3600,
	})
	if err != nil {
		return R2Creds{}, err
	}
	body := header + "." + b64url(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	token := body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	sum := sha256.Sum256([]byte(token))
	return R2Creds{
		AccessKeyID:     accessKey,
		SecretAccessKey: hex.EncodeToString(sum[:]),
		SessionToken:    base64.StdEncoding.EncodeToString([]byte("jwt/" + token)),
	}, nil
}

func b64url(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

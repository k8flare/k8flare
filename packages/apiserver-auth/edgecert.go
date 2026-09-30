package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	x509request "k8s.io/apiserver/pkg/authentication/request/x509"
	"k8s.io/apiserver/pkg/authentication/user"
)

const ClientCertHeader = "X-K8flare-Client-Cert"

type EdgeClientCert struct {
	ClientCA func(context.Context) ([]byte, error)
}

func (e EdgeClientCert) AuthenticateRequest(r *http.Request) (*authenticator.Response, bool, error) {
	raw := r.Header.Get(ClientCertHeader)
	if raw == "" {
		return nil, false, nil
	}
	leaf, err := parseRFC9440(raw)
	if err != nil {
		return nil, false, err
	}
	caPEM, err := e.ClientCA(r.Context())
	if err != nil {
		return nil, false, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, false, errors.New("client CA is not a certificate")
	}
	opts := x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	verified := r.Clone(r.Context())
	verified.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
	resp, ok, err := x509request.New(opts, x509request.CommonNameUserConversion).AuthenticateRequest(verified)
	if err != nil || !ok {
		return nil, false, err
	}
	info := &user.DefaultInfo{
		Name:   resp.User.GetName(),
		UID:    resp.User.GetUID(),
		Groups: append(append([]string(nil), resp.User.GetGroups()...), user.AllAuthenticated),
		Extra:  resp.User.GetExtra(),
	}
	return &authenticator.Response{User: info}, true, nil
}

func parseRFC9440(value string) (*x509.Certificate, error) {
	encoded, ok := strings.CutPrefix(value, ":")
	if ok {
		encoded, ok = strings.CutSuffix(encoded, ":")
	}
	if !ok {
		return nil, errors.New("client certificate is not RFC 9440 encoded")
	}
	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

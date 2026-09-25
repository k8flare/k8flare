package admission

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	certificatesv1 "k8s.io/api/certificates/v1"
)

func applyCertificateSubjectRestriction(_ context.Context, _ *store, req *admit.Request) error {
	if req.Resource.Resource != "certificatesigningrequests" || req.Subresource != "" {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	if req.Object == nil {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	signer, _ := spec["signerName"].(string)
	if signer != certificatesv1.KubeAPIServerClientSignerName {
		return nil
	}
	csr, err := parseCSRRequest(spec["request"])
	if err != nil {
		return fmt.Errorf("failed to parse CSR: %v", err)
	}
	for _, group := range csr.Subject.Organization {
		if group == "system:masters" {
			return fmt.Errorf("use of %s signer with system:masters group is not allowed", certificatesv1.KubeAPIServerClientSignerName)
		}
	}
	return nil
}

func parseCSRRequest(raw any) (*x509.CertificateRequest, error) {
	pemBytes, ok := csrPEMBytes(raw)
	if !ok {
		return nil, fmt.Errorf("PEM block type must be CERTIFICATE REQUEST")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("PEM block type must be CERTIFICATE REQUEST")
	}
	return x509.ParseCertificateRequest(block.Bytes)
}

func csrPEMBytes(raw any) ([]byte, bool) {
	var b []byte
	switch v := raw.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return nil, false
	}
	if len(b) == 0 {
		return nil, false
	}
	if block, _ := pem.Decode(b); block != nil {
		return b, true
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(dec) == 0 {
		return b, true
	}
	return dec, true
}

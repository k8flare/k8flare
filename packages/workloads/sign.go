package workloads

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"

	capi "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	certificatesinformers "k8s.io/client-go/informers/certificates/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/cert"
	csrcert "k8s.io/client-go/util/certificate/csr"
	"k8s.io/client-go/util/keyutil"
	capihelper "k8s.io/kubernetes/pkg/apis/certificates"
	"k8s.io/kubernetes/pkg/controller/certificates"
	"k8s.io/kubernetes/pkg/controller/certificates/authority"
)

const clusterSigningTTL = 365 * 24 * time.Hour

func VaultPEM(ctx context.Context, store *kine.Client, name string) ([]byte, error) {
	kv, _, err := store.Get(ctx, "/vault/ca/"+name)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, err
	}
	var record struct {
		Cert string `json:"cert"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	if record.Cert == "" || record.Key == "" {
		return nil, fmt.Errorf("vault ca %s is incomplete", name)
	}
	return []byte(record.Cert + record.Key), nil
}

var ServiceAccountKey []byte

func VaultServiceAccountKey(ctx context.Context, store *kine.Client) ([]byte, error) {
	kv, _, err := store.Get(ctx, "/vault/sa-signing-key")
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func startCSRSigners(ctx context.Context, client kubernetes.Interface, inf certificatesinformers.CertificateSigningRequestInformer, clientCA, servingCA []byte) []func(context.Context) {
	var runs []func(context.Context)
	add := func(name string, pem []byte, signers ...string) {
		cc, err := newCSRSigningController(ctx, client, inf, name, pem, signers...)
		if err != nil {
			println("workloads:", name+":", err.Error())
			return
		}
		if cc != nil {
			runs = append(runs, func(ctx context.Context) { cc.Run(ctx, workers) })
		}
	}
	add("csrsigning-kubelet-client", clientCA, capi.KubeAPIServerClientKubeletSignerName, capi.KubeAPIServerClientSignerName)
	add("csrsigning-kubelet-serving", servingCA, capi.KubeletServingSignerName)
	return runs
}

func newCSRSigningController(ctx context.Context, client kubernetes.Interface, inf certificatesinformers.CertificateSigningRequestInformer, name string, signingCA []byte, signers ...string) (*certificates.CertificateController, error) {
	if len(signingCA) == 0 {
		return nil, nil
	}
	ca, err := signingAuthority(signingCA)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, s := range signers {
		allowed[s] = true
	}
	s := &csrSigner{client: client, ca: ca, certTTL: clusterSigningTTL, signers: allowed}
	return certificates.NewCertificateController(ctx, name, client, inf, s.handle), nil
}

type csrSigner struct {
	client  kubernetes.Interface
	ca      *authority.CertificateAuthority
	certTTL time.Duration
	signers map[string]bool
}

func (s *csrSigner) handle(ctx context.Context, csr *capi.CertificateSigningRequest) error {
	if !certificates.IsCertificateRequestApproved(csr) || certificates.HasTrueCondition(csr, capi.CertificateFailed) {
		return nil
	}
	if !s.signers[csr.Spec.SignerName] {
		return nil
	}
	x509cr, err := capihelper.ParseCSR(csr.Spec.Request)
	if err != nil {
		return fmt.Errorf("unable to parse csr %q: %v", csr.Name, err)
	}
	if err := signerUsagesOK(x509cr, csr.Spec.Usages, csr.Spec.SignerName); err != nil {
		csr.Status.Conditions = append(csr.Status.Conditions, capi.CertificateSigningRequestCondition{
			Type:           capi.CertificateFailed,
			Status:         v1.ConditionTrue,
			Reason:         "SignerValidationFailure",
			Message:        err.Error(),
			LastUpdateTime: metav1.Now(),
		})
		_, err = s.client.CertificatesV1().CertificateSigningRequests().UpdateStatus(ctx, csr, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("error adding failure condition for csr: %v", err)
		}
		return nil
	}
	certPEM, err := s.sign(x509cr, csr.Spec.Usages, csr.Spec.ExpirationSeconds)
	if err != nil {
		return fmt.Errorf("error auto signing csr: %v", err)
	}
	csr.Status.Certificate = certPEM
	_, err = s.client.CertificatesV1().CertificateSigningRequests().UpdateStatus(ctx, csr, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("error updating signature for csr: %v", err)
	}
	return nil
}

func (s *csrSigner) sign(x509cr *x509.CertificateRequest, usages []capi.KeyUsage, expirationSeconds *int32) ([]byte, error) {
	der, err := s.ca.Sign(x509cr.Raw, authority.PermissiveSigningPolicy{
		TTL:      signingDuration(s.certTTL, expirationSeconds),
		Usages:   usages,
		Backdate: 5 * time.Minute,
		Short:    8 * time.Hour,
	})
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

func signingDuration(certTTL time.Duration, expirationSeconds *int32) time.Duration {
	if expirationSeconds == nil {
		return certTTL
	}
	const min = 10 * time.Minute
	switch requested := csrcert.ExpirationSecondsToDuration(*expirationSeconds); {
	case requested > certTTL:
		return certTTL
	case requested < min:
		return min
	default:
		return requested
	}
}

func signerUsagesOK(req *x509.CertificateRequest, usages []capi.KeyUsage, signerName string) error {
	switch signerName {
	case capi.KubeAPIServerClientKubeletSignerName:
		return capihelper.ValidateKubeletClientCSR(req, usagesToSet(usages))
	case capi.KubeAPIServerClientSignerName:
		return validAPIServerClientUsages(usages)
	case capi.KubeletServingSignerName:
		return capihelper.ValidateKubeletServingCSR(req, usagesToSet(usages))
	default:
		return fmt.Errorf("unrecognized signerName: %q", signerName)
	}
}

func validAPIServerClientUsages(usages []capi.KeyUsage) error {
	hasClientAuth := false
	for _, u := range usages {
		switch u {
		case capi.UsageDigitalSignature, capi.UsageKeyEncipherment:
		case capi.UsageClientAuth:
			hasClientAuth = true
		default:
			return fmt.Errorf("invalid usage for client certificate: %s", u)
		}
	}
	if !hasClientAuth {
		return fmt.Errorf("missing required usage for client certificate: %s", capi.UsageClientAuth)
	}
	return nil
}

func usagesToSet(usages []capi.KeyUsage) sets.String {
	result := sets.NewString()
	for _, usage := range usages {
		result.Insert(string(usage))
	}
	return result
}

func signingAuthority(pemBytes []byte) (*authority.CertificateAuthority, error) {
	certs, err := cert.ParseCertsPEM(pemBytes)
	if err != nil {
		return nil, err
	}
	if len(certs) != 1 {
		return nil, fmt.Errorf("expected 1 certificate, found %d", len(certs))
	}
	key, err := keyutil.ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		return nil, err
	}
	priv, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("key did not implement crypto.Signer")
	}
	return &authority.CertificateAuthority{RawCert: pemBytes, RawKey: pemBytes, Certificate: certs[0], PrivateKey: priv}, nil
}

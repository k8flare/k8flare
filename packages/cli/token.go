package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
)

type tokenRecord struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Token       string     `json:"token"`
}

func runToken(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: k8flare token create|list|delete|revoke|rotate")
	}
	switch args[0] {
	case "create":
		return tokenCreate(args[1:], out)
	case "list":
		return tokenList(args[1:], out)
	case "delete", "revoke":
		return tokenDelete(args[1:], out)
	case "rotate":
		return tokenRotate(args[1:], out)
	}
	return fmt.Errorf("unknown token command %q", args[0])
}

func tokenCreate(args []string, out io.Writer) error {
	cmd := newCommand("token create")
	ttl := cmd.fs.Duration("ttl", 24*time.Hour, "time before the token expires; 0 never expires")
	description := cmd.fs.String("description", "", "what the token is for")
	bootstrap := cmd.fs.Bool("bootstrap", false, "write a bootstrap-token Secret instead of a vault token")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	if *ttl < 0 {
		return errors.New("--ttl must not be negative")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	caHash, err := c.serverCAHash()
	if err != nil {
		return err
	}
	if *bootstrap {
		token, err := c.createBootstrapToken(*description, *ttl)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, k10Token(caHash, token))
		return nil
	}
	var created tokenRecord
	body := map[string]any{"description": *description, "ttlSeconds": int64(ttl.Seconds())}
	if err := c.call("POST", "/internal/tokens", nil, body, &created); err != nil {
		return err
	}
	fmt.Fprintln(out, k10Token(caHash, created.Token))
	return nil
}

func (c *client) createBootstrapToken(description string, ttl time.Duration) (string, error) {
	token, err := bootstraputil.GenerateBootstrapToken()
	if err != nil {
		return "", err
	}
	id, secret, _ := strings.Cut(token, ".")
	data := map[string][]byte{
		bootstrapapi.BootstrapTokenIDKey:               []byte(id),
		bootstrapapi.BootstrapTokenSecretKey:           []byte(secret),
		bootstrapapi.BootstrapTokenUsageAuthentication: []byte("true"),
	}
	if description != "" {
		data[bootstrapapi.BootstrapTokenDescriptionKey] = []byte(description)
	}
	if ttl > 0 {
		data[bootstrapapi.BootstrapTokenExpirationKey] = []byte(time.Now().Add(ttl).UTC().Format(time.RFC3339))
	}
	body := corev1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: bootstraputil.BootstrapTokenSecretName(id), Namespace: metav1.NamespaceSystem},
		Type:       bootstrapapi.SecretTypeBootstrapToken,
		Data:       data,
	}
	if err := c.call("POST", "/api/v1/namespaces/"+metav1.NamespaceSystem+"/secrets", nil, body, nil); err != nil {
		return "", err
	}
	return token, nil
}

func tokenList(args []string, out io.Writer) error {
	cmd := newCommand("token list")
	if _, err := cmd.parse(args); err != nil {
		return err
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	var listed struct {
		Items []tokenRecord `json:"items"`
	}
	if err := c.call("GET", "/internal/tokens", nil, nil, &listed); err != nil {
		return err
	}
	var secrets corev1.SecretList
	selector := url.Values{"fieldSelector": {"type=" + string(bootstrapapi.SecretTypeBootstrapToken)}}
	if err := c.call("GET", "/api/v1/namespaces/"+metav1.NamespaceSystem+"/secrets", selector, nil, &secrets); err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 10, 4, 3, ' ', 0)
	fmt.Fprintf(w, "ID\tTYPE\tEXPIRES\tDESCRIPTION\n")
	for _, t := range listed.Items {
		expires := "<never>"
		if t.ExpiresAt != nil {
			expires = t.ExpiresAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\tvault\t%s\t%s\n", t.ID, expires, orNone(t.Description))
	}
	for _, secret := range secrets.Items {
		expires := string(secret.Data[bootstrapapi.BootstrapTokenExpirationKey])
		if expires == "" {
			expires = "<never>"
		}
		description := string(secret.Data[bootstrapapi.BootstrapTokenDescriptionKey])
		fmt.Fprintf(w, "%s\tbootstrap\t%s\t%s\n", secret.Data[bootstrapapi.BootstrapTokenIDKey], expires, orNone(description))
	}
	return w.Flush()
}

func orNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

func bootstrapSecretPath(id string) string {
	return "/api/v1/namespaces/" + metav1.NamespaceSystem + "/secrets/" + bootstraputil.BootstrapTokenSecretName(id)
}

func (c *client) serverCAHash() (string, error) {
	pem, err := c.do("GET", "/cacerts", nil, nil)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pem)
	return hex.EncodeToString(sum[:]), nil
}

func k10Token(caHash, token string) string {
	return "K10" + caHash + "::" + token
}

func tokenID(arg string) string {
	if _, creds, ok := strings.Cut(arg, "::"); ok {
		arg = creds
	}
	id, _, _ := strings.Cut(arg, ".")
	return id
}

func tokenDelete(args []string, out io.Writer) error {
	cmd := newCommand("token delete")
	ids, err := cmd.parse(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("usage: k8flare token delete <id|token>...")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	for _, arg := range ids {
		id := tokenID(arg)
		vaultErr := c.call("DELETE", "/internal/tokens/"+id, nil, nil, nil)
		if vaultErr != nil && !isNotFound(vaultErr) {
			return vaultErr
		}
		if vaultErr == nil {
			fmt.Fprintf(out, "token %s deleted\n", id)
		}
		bootstrapErr := c.call("DELETE", bootstrapSecretPath(id), nil, nil, nil)
		if bootstrapErr != nil && !isNotFound(bootstrapErr) {
			return bootstrapErr
		}
		if bootstrapErr == nil {
			fmt.Fprintf(out, "bootstrap token %s deleted\n", id)
		}
		if vaultErr != nil && bootstrapErr != nil {
			return fmt.Errorf("token %s not found: no vault token and no %s Secret in %s", id, bootstraputil.BootstrapTokenSecretName(id), metav1.NamespaceSystem)
		}
	}
	return nil
}

func tokenRotate(args []string, out io.Writer) error {
	cmd := newCommand("token rotate")
	ids, err := cmd.parse(args)
	if err != nil {
		return err
	}
	if len(ids) != 1 {
		return errors.New("usage: k8flare token rotate <id|token>")
	}
	c, err := cmd.conn.dial()
	if err != nil {
		return err
	}
	caHash, err := c.serverCAHash()
	if err != nil {
		return err
	}
	id := tokenID(ids[0])
	var rotated tokenRecord
	if err := c.call("POST", "/internal/tokens/"+id+"/rotate", nil, nil, &rotated); err == nil {
		fmt.Fprintln(out, k10Token(caHash, rotated.Token))
		return nil
	} else if !isNotFound(err) {
		return err
	}
	var bootstrapToken corev1.Secret
	if err := c.call("GET", bootstrapSecretPath(id), nil, nil, &bootstrapToken); err != nil {
		return err
	}
	newToken, err := bootstraputil.GenerateBootstrapToken()
	if err != nil {
		return err
	}
	_, secret, _ := strings.Cut(newToken, ".")
	bootstrapToken.Data[bootstrapapi.BootstrapTokenSecretKey] = []byte(secret)
	if expires, err := time.Parse(time.RFC3339, string(bootstrapToken.Data[bootstrapapi.BootstrapTokenExpirationKey])); err == nil && !bootstrapToken.CreationTimestamp.IsZero() {
		bootstrapToken.Data[bootstrapapi.BootstrapTokenExpirationKey] = []byte(time.Now().Add(expires.Sub(bootstrapToken.CreationTimestamp.Time)).UTC().Format(time.RFC3339))
	}
	if err := c.call("PUT", bootstrapSecretPath(id), nil, &bootstrapToken, nil); err != nil {
		return err
	}
	fmt.Fprintln(out, k10Token(caHash, id+"."+secret))
	return nil
}

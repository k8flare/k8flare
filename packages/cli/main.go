package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const usage = `usage: k8flare <command>

commands:
  token create|list|delete|rotate
  certificate rotate-ca|check
  edge-certificate --hosts <name,...> [--out-dir <dir>] [--ttl <duration>]
  secrets-encrypt status|reencrypt
  snapshot save|list|restore
  restore --to <time>

every command reads the cluster and admin credential from --kubeconfig
(default: $KUBECONFIG or ~/.kube/config), or from --server and --token.`

type connection struct {
	kubeconfig string
	context    string
	server     string
	token      string
	insecure   bool
}

func (c *connection) bind(fs *flag.FlagSet) {
	fs.StringVar(&c.kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	fs.StringVar(&c.context, "context", "", "kubeconfig context to use")
	fs.StringVar(&c.server, "server", "", "cluster URL, overrides the kubeconfig")
	fs.StringVar(&c.token, "token", "", "admin token, overrides the kubeconfig")
	fs.BoolVar(&c.insecure, "insecure-skip-tls-verify", false, "do not verify the server certificate")
}

type client struct {
	http *http.Client
	base string
}

func (c *connection) dial() (*client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = c.kubeconfig
	overrides := &clientcmd.ConfigOverrides{CurrentContext: c.context}
	if c.server != "" {
		overrides.ClusterInfo.Server = c.server
	}
	if c.insecure {
		overrides.ClusterInfo.InsecureSkipTLSVerify = true
	}
	if c.token != "" {
		overrides.AuthInfo.Token = c.token
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(config.Host, "/")
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	return &client{http: httpClient, base: base}, nil
}

func (c *client) call(method, path string, query url.Values, body, out any) error {
	data, err := c.do(method, path, query, body)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *client) do(method, path string, query url.Values, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(data)
	}
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, target, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, serverMessage(data))
	}
	return data, nil
}

func serverMessage(data []byte) string {
	var failure struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &failure) == nil && failure.Error != "" {
		return failure.Error
	}
	return strings.TrimSpace(string(data))
}

func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

type command struct {
	fs   *flag.FlagSet
	conn connection
}

func newCommand(name string) *command {
	c := &command{fs: flag.NewFlagSet(name, flag.ContinueOnError)}
	c.fs.SetOutput(io.Discard)
	c.conn.bind(c.fs)
	return c
}

func (c *command) parse(args []string) ([]string, error) {
	return parseInterspersed(c.fs, args)
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "token":
		return runToken(args[1:], out)
	case "certificate":
		return runCertificate(args[1:], out)
	case "edge-certificate":
		return runEdgeCertificate(args[1:], out)
	case "secrets-encrypt":
		return runSecretsEncrypt(args[1:], out)
	case "snapshot":
		return runSnapshot(args[1:], out)
	case "restore":
		return runRestore(args[1:], out)
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "k8flare:", err)
		os.Exit(1)
	}
}

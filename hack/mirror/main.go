// Command mirror copies pinned upstream Go modules into .build/ and applies
// the overlays that make them build for GOOS=js. Root go.mod's replace
// directives point at the mirrors, so this must run before any Go build.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type op struct {
	kind    string
	path    string
	overlay string
	from    string
	to      string
	text    string
	edits   []op
}

func patch(path, from, to string) op { return op{kind: "patch", path: path, from: from, to: to} }

// hostOnly keeps the upstream file for every target but js.
func hostOnly(path string) op { return op{kind: "hostOnly", path: path} }

// replaceJS keeps the upstream file for host builds and adds the overlay
// (which carries its own //go:build js constraint) beside it.
func replaceJS(path, overlay string) op { return op{kind: "replaceJS", path: path, overlay: overlay} }

// patchJS keeps the upstream file for host builds and adds a js-only copy
// with the given replacements applied and text appended.
func patchJS(path string, edits []op) op { return op{kind: "patchJS", path: path, edits: edits} }

func appendText(path, text string) op { return op{kind: "append", path: path, text: text} }

type mirror struct {
	name    string
	module  string
	version string
	pins    []string
	ops     []op
}

var mirrors = []mirror{
	{
		name:    "k3s",
		module:  "github.com/k3s-io/k3s",
		version: "v1.36.5-0.20260821152713-4dedb15be780",
		pins:    []string{"pkg/daemons/control/deps/deps.go"},
		ops: []op{
			patch("pkg/daemons/control/deps/deps.go",
				"func KubeConfig(dest, url, caCert, clientCert, clientKey string) error {\n",
				"func KubeConfig(dest, url, caCert, clientCert, clientKey string) error {\n\tif KubeConfigOverride != nil {\n\t\tif handled, err := KubeConfigOverride(dest, url, caCert, clientCert, clientKey); handled || err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"),
			appendText("pkg/daemons/control/deps/deps.go", `
// KubeConfigOverride lets an embedding program write the agent's
// kubeconfigs itself. k8flare's control plane sits behind a TLS terminator
// that never sees client certificates, so cmd/agent writes bearer-token
// kubeconfigs instead of the certificate ones above. Added by hack/mirror.
var KubeConfigOverride func(dest, url, caCert, clientCert, clientKey string) (handled bool, err error)
`),
		},
	},
	{
		name:    "component-base",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/component-base",
		version: "v1.36.4-k3s1",
		pins:    []string{"tracing/utils.go"},
		ops: []op{
			replaceJS("tracing/utils.go", "component-base/tracing_utils.go"),
		},
	},
	{
		name:    "apiserver",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/apiserver",
		version: "v1.36.4-k3s1",
		pins: []string{
			"pkg/storage/storagebackend/config.go",
			"pkg/storage/storagebackend/factory/factory.go",
			"pkg/storage/feature/feature_support_checker.go",
			"pkg/sharding/parser.go",
			"pkg/endpoints/installer.go",
		},
		ops: []op{
			hostOnly("pkg/storage/storagebackend/factory/etcd3.go"),
			replaceJS("pkg/storage/storagebackend/factory/factory.go", "apiserver/factory.go"),
			replaceJS("pkg/storage/feature/feature_support_checker.go", "apiserver/feature_support_checker.go"),
			replaceJS("pkg/sharding/parser.go", "apiserver/sharding_parser.go"),
			patchJS("pkg/storage/storagebackend/config.go", []op{
				patch("", "\t\"k8s.io/apiserver/pkg/server/egressselector\"\n", ""),
				patch("", "\t\"k8s.io/apiserver/pkg/storage/etcd3\"\n", ""),
				patch("", "\tEgressLookup egressselector.Lookup\n", ""),
				patch("", "\tLeaseManagerConfig etcd3.LeaseManagerConfig\n", "\tLeaseManagerConfig LeaseManagerConfig\n"),
				patch("", "etcd3.NewDefaultLeaseManagerConfig()", "NewDefaultLeaseManagerConfig()"),
				appendText("", `
// Local copy of etcd3.LeaseManagerConfig so that this package does not link
// the etcd3 storage implementation and the etcd client, which do not build
// for GOOS=js. Added by hack/mirror.
type LeaseManagerConfig struct {
	ReuseDurationSeconds int64
	MaxObjectCount       int64
}

func NewDefaultLeaseManagerConfig() LeaseManagerConfig {
	return LeaseManagerConfig{ReuseDurationSeconds: 60, MaxObjectCount: 1000}
}
`),
			}),
			patch("pkg/endpoints/installer.go",
				"\t\tHubGroupVersion: schema.GroupVersion{Group: fqKindToRegister.Group, Version: runtime.APIVersionInternal},",
				"\t\tHubGroupVersion: hubGroupVersionFor(a.group.Typer, a.group.GroupVersion, fqKindToRegister),"),
			appendText("pkg/endpoints/installer.go", `
// hubGroupVersionFor is the version a PATCH body is decoded to before the
// merge is applied: the internal version when the scheme has one, the served
// version otherwise. This apiserver registers external types only, and
// upstream hardcodes the internal hub. Added by hack/mirror.
func hubGroupVersionFor(typer runtime.ObjectTyper, served schema.GroupVersion, kind schema.GroupVersionKind) schema.GroupVersion {
	internal := schema.GroupVersion{Group: kind.Group, Version: runtime.APIVersionInternal}
	if typer != nil && typer.Recognizes(internal.WithKind(kind.Kind)) {
		return internal
	}
	return served
}
`),
		},
	},
}

func main() {
	writePins := len(os.Args) > 1 && os.Args[1] == "-write-pins"
	root, err := repoRoot()
	check(err)
	for _, m := range mirrors {
		src, err := moduleDir(m.module, m.version)
		check(err)
		dst := filepath.Join(root, ".build", m.name+"-mirror")
		overlays := filepath.Join(root, "hack/mirror/_overlays")
		for _, rel := range m.pins {
			check(checkPin(src, rel, filepath.Join(overlays, m.name, pinName(rel)), writePins))
		}
		check(os.RemoveAll(dst))
		check(copyTree(src, dst))
		for _, o := range m.ops {
			check(apply(dst, overlays, o))
		}
		fmt.Printf("mirror: %s %s@%s -> %s\n", m.name, m.module, m.version, dst)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func moduleDir(module, version string) (string, error) {
	cmd := exec.Command("go", "mod", "download", "-json", module+"@"+version)
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GO111MODULE=on")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go mod download %s@%s: %w", module, version, err)
	}
	var info struct{ Dir string }
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	if info.Dir == "" {
		return "", fmt.Errorf("go mod download %s@%s: no Dir", module, version)
	}
	return info.Dir, nil
}

func pinName(rel string) string {
	return "upstream-" + strings.ReplaceAll(rel, "/", "-") + ".sha256"
}

func checkPin(src, rel, pinFile string, write bool) error {
	data, err := os.ReadFile(filepath.Join(src, rel))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if write {
		return os.WriteFile(pinFile, []byte(got+"\n"), 0o644)
	}
	want, err := os.ReadFile(pinFile)
	if err != nil {
		return fmt.Errorf("%s: no pin (run with -write-pins after reviewing the overlay): %w", rel, err)
	}
	if strings.TrimSpace(string(want)) != got {
		return fmt.Errorf("%s changed upstream (pin %s, got %s): review the overlay, then refresh the pin", rel, strings.TrimSpace(string(want)), got)
	}
	return nil
}

func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return os.CopyFS(dst, os.DirFS(src))
}

const hostTag = "//go:build !js\n\n"
const jsTag = "//go:build js\n\n"

func jsName(path string) string {
	return strings.TrimSuffix(path, ".go") + "_js.go"
}

// keepHostOnly constrains the upstream file to every target but js and
// returns its original content.
func keepHostOnly(dst, path string) ([]byte, error) {
	target := filepath.Join(dst, path)
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	return data, os.WriteFile(target, append([]byte(hostTag), data...), 0o644)
}

func apply(dst, overlays string, o op) error {
	target := filepath.Join(dst, o.path)
	switch o.kind {
	case "hostOnly":
		_, err := keepHostOnly(dst, o.path)
		return err
	case "replaceJS":
		if _, err := keepHostOnly(dst, o.path); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(overlays, o.overlay))
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(data, []byte("//go:build js")) {
			return fmt.Errorf("%s: overlay must start with a //go:build js constraint", o.overlay)
		}
		return os.WriteFile(filepath.Join(dst, jsName(o.path)), data, 0o644)
	case "patchJS":
		data, err := keepHostOnly(dst, o.path)
		if err != nil {
			return err
		}
		for _, e := range o.edits {
			switch e.kind {
			case "patch":
				if !bytes.Contains(data, []byte(e.from)) {
					return fmt.Errorf("%s no longer contains the text this patch replaces:\n%s", o.path, e.from)
				}
				data = bytes.Replace(data, []byte(e.from), []byte(e.to), 1)
			case "append":
				data = append(data, e.text...)
			}
		}
		return os.WriteFile(filepath.Join(dst, jsName(o.path)), append([]byte(jsTag), data...), 0o644)
	case "patch":
		data, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(o.from)) {
			return fmt.Errorf("%s no longer contains the text this patch replaces:\n%s", o.path, o.from)
		}
		return os.WriteFile(target, bytes.Replace(data, []byte(o.from), []byte(o.to), 1), 0o644)
	case "append":
		f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(o.text)
		return err
	}
	return fmt.Errorf("unknown op %q", o.kind)
}

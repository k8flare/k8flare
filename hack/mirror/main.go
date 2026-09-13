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
	"io"
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
}

func del(path string) op              { return op{kind: "delete", path: path} }
func replace(path, overlay string) op { return op{kind: "replace", path: path, overlay: overlay} }
func patch(path, from, to string) op  { return op{kind: "patch", path: path, from: from, to: to} }
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
		name:    "component-base",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/component-base",
		version: "v1.36.4-k3s1",
		pins:    []string{"tracing/utils.go"},
		ops: []op{
			replace("tracing/utils.go", "component-base/tracing_utils.go"),
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
			del("pkg/storage/storagebackend/factory/etcd3.go"),
			del("pkg/storage/storagebackend/factory/etcd3_test.go"),
			del("pkg/storage/storagebackend/factory/factory_test.go"),
			del("pkg/storage/storagebackend/factory/tls_test.go"),
			replace("pkg/storage/storagebackend/factory/factory.go", "apiserver/factory.go"),
			del("pkg/storage/feature/feature_support_checker_test.go"),
			replace("pkg/storage/feature/feature_support_checker.go", "apiserver/feature_support_checker.go"),
			del("pkg/sharding/parser_test.go"),
			replace("pkg/sharding/parser.go", "apiserver/sharding_parser.go"),
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
			patch("pkg/storage/storagebackend/config.go", "\t\"k8s.io/apiserver/pkg/server/egressselector\"\n", ""),
			patch("pkg/storage/storagebackend/config.go", "\t\"k8s.io/apiserver/pkg/storage/etcd3\"\n", ""),
			patch("pkg/storage/storagebackend/config.go", "\tEgressLookup egressselector.Lookup\n", ""),
			patch("pkg/storage/storagebackend/config.go", "\tLeaseManagerConfig etcd3.LeaseManagerConfig\n", "\tLeaseManagerConfig LeaseManagerConfig\n"),
			patch("pkg/storage/storagebackend/config.go", "etcd3.NewDefaultLeaseManagerConfig()", "NewDefaultLeaseManagerConfig()"),
			appendText("pkg/storage/storagebackend/config.go", `
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
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

func apply(dst, overlays string, o op) error {
	target := filepath.Join(dst, o.path)
	switch o.kind {
	case "delete":
		return os.Remove(target)
	case "replace":
		data, err := os.ReadFile(filepath.Join(overlays, o.overlay))
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
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

package main

import (
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// writeGoFile formats src as Go source (failing loudly on a syntax error
// rather than writing broken output) and writes it to path, creating parent
// directories as needed.
func writeGoFile(path string, src []byte) error {
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("format %s: %w", path, err)
	}
	return writeFile(path, formatted)
}

// writeFile writes data to path, creating parent directories as needed.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// goListModuleDir returns the on-disk directory Go resolves the given
// module path to (following go.mod replace directives), by shelling out to
// `go list -m`. Used to locate the k3s-io/kubernetes fork's checked-in
// api/openapi-spec/ files, which aren't importable as a Go package so
// there's no other way to find them from within a Go program.
func goListModuleDir(root, modulePath string) (string, error) {
	out, err := runGo(root, "list", "-m", "-f", "{{.Dir}}", modulePath)
	if err != nil {
		return "", fmt.Errorf("locate module %s: %w", modulePath, err)
	}
	return strings.TrimSpace(out), nil
}

// goListModuleVersion returns the resolved version Go uses for the given
// module path (following go.mod replace directives).
func goListModuleVersion(root, modulePath string) (string, error) {
	out, err := runGo(root, "list", "-m", "-f", "{{.Version}}", modulePath)
	if err != nil {
		return "", fmt.Errorf("resolve version of %s: %w", modulePath, err)
	}
	return strings.TrimSpace(out), nil
}

func runGo(root string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, ee.Stderr)
		}
		return "", fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

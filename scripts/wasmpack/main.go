// Command wasmpack prepares a Go WASM binary for the Worker Loader:
//
//	wasmpack exec <out.js>                 patch $GOROOT/lib/wasm/wasm_exec.js
//	wasmpack chunk <in.wasm> <dir> <name>  split into <=24MiB parts + manifest
//
// The Loader receives the binary as one module, but Static Assets cap a
// file at 25MiB, so the binary ships in parts that the loader factory
// reassembles. The manifest's sha256 is the Loader isolate cache key.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "exec":
		if len(os.Args) != 3 {
			usage()
		}
		err = patchExec(os.Args[2])
	case "chunk":
		if len(os.Args) != 5 {
			usage()
		}
		err = chunk(os.Args[2], os.Args[3], os.Args[4])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wasmpack:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wasmpack exec <out.js> | wasmpack chunk <in.wasm> <dir> <name>")
	os.Exit(2)
}

const chunkBytes = 24 * 1024 * 1024

type manifest struct {
	Size   int      `json:"size"`
	SHA256 string   `json:"sha256"`
	Parts  []string `json:"parts"`
}

func chunk(input, dir, name string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, name+".wasm.part*"))
	for _, f := range old {
		os.Remove(f)
	}
	sum := sha256.Sum256(data)
	m := manifest{Size: len(data), SHA256: hex.EncodeToString(sum[:])}
	for off, i := 0, 0; off < len(data); off, i = off+chunkBytes, i+1 {
		end := min(off+chunkBytes, len(data))
		part := fmt.Sprintf("%s.wasm.part%d", name, i)
		if err := os.WriteFile(filepath.Join(dir, part), data[off:end], 0o644); err != nil {
			return err
		}
		m.Parts = append(m.Parts, part)
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name+".manifest.json"), append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wasmpack: %s %d bytes -> %d parts (sha256 %s)\n", name, len(data), len(m.Parts), m.SHA256[:16])
	return nil
}

// patchExec threads a per-invocation `context` object through Go.run and
// exposes it as globalThis.context via a Proxy, so Go code reached through
// syscall/js can find the request's env and bindings. fetch and setTimeout
// are re-bound to the real global because both brand-check their receiver.
func patchExec(out string) error {
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	src, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(goroot)), "lib/wasm/wasm_exec.js"))
	if err != nil {
		return err
	}
	s := string(src)
	replaceOnce := func(from, to string) error {
		if strings.Count(s, from) != 1 {
			return fmt.Errorf("expected exactly one %q in wasm_exec.js; upstream changed, review this patch", from)
		}
		s = strings.Replace(s, from, to, 1)
		return nil
	}
	if err := replaceOnce("async run(instance) {", "async run(instance, context) {"); err != nil {
		return err
	}
	valuesAnchor := "this._values = [ // JS values that Go currently has references to, indexed by reference id"
	proxy := `const boundGlobals = new Set(["fetch", "setTimeout"]);
			const globalProxy = new Proxy(globalThis, {
				get(target, prop) {
					if (prop === "context") return context;
					const val = Reflect.get(target, prop, target);
					if (boundGlobals.has(prop) && typeof val === "function") return val.bind(target);
					return val;
				},
			});
			`
	if err := replaceOnce(valuesAnchor, proxy+valuesAnchor); err != nil {
		return err
	}
	for _, re := range []*regexp.Regexp{regexp.MustCompile(`(?m)^(\t{4})globalThis,$`), regexp.MustCompile(`(?m)^(\t{4})\[globalThis, 5\],$`)} {
		if len(re.FindAllStringIndex(s, -1)) != 1 {
			return fmt.Errorf("expected exactly one %s inside run(); upstream changed, review this patch", re)
		}
		s = re.ReplaceAllStringFunc(s, func(m string) string { return strings.Replace(m, "globalThis", "globalProxy", 1) })
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, []byte(s), 0o644)
}

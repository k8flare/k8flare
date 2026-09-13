// Command e2e downloads upstream's e2e.test binary for the pinned
// Kubernetes version and runs it against the local cluster with a focus
// regex narrowed to what a single-node k3s-on-Workers deployment can pass.
//
//	go run ./e2e [-set required|advisory|all] [-focus <regex override>]
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/k8flare/k8flare/scripts/internal/upstream"
)

func main() {
	set := flag.String("set", "required", "test set to run: required, advisory, or all")
	focus := flag.String("focus", "", "override the built-in focus regex")
	procs := flag.Int("procs", 4, "parallel ginkgo processes; [Serial] specs still run alone")
	flag.Parse()

	root, err := upstream.RepoRoot()
	check(err)

	kubeconfig := filepath.Join(root, ".build/kubeconfig.yaml")
	if _, err := os.Stat(kubeconfig); err != nil {
		log.Fatal("no .build/kubeconfig.yaml; run make kubeconfig")
	}
	check(preflight(kubeconfig))

	e2eTest, err := ensureE2ETest(root)
	check(err)

	for _, s := range setsToRun(*set) {
		regex := *focus
		if regex == "" {
			regex = focusRegex(sets[s])
		}
		reportDir := filepath.Join(root, ".build/e2e/report", s)
		check(os.MkdirAll(reportDir, 0o755))
		fmt.Printf("=== e2e set %q ===\n", s)
		err := runE2E(e2eTest, kubeconfig, regex, reportDir, *procs)
		if err != nil && s == "required" {
			log.Fatalf("required e2e set failed: %v", err)
		}
		if err != nil {
			fmt.Printf("advisory e2e set %q failed: %v\n", s, err)
		}
	}
}

func setsToRun(set string) []string {
	if set == "all" {
		return []string{"required", "advisory"}
	}
	if _, ok := sets[set]; !ok {
		log.Fatalf("unknown -set %q; want required, advisory, or all", set)
	}
	return []string{set}
}

func focusRegex(names []string) string {
	escaped := make([]string, len(names))
	for i, name := range names {
		escaped[i] = regexp.QuoteMeta(name)
	}
	return strings.Join(escaped, "|")
}

func preflight(kubeconfig string) error {
	out, err := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
		"-o", `jsonpath={range .items[*]}{.metadata.name}{" "}{range .status.conditions[?(@.type=="Ready")]}{.status}{end}{"\n"}{end}`,
	).Output()
	if err != nil {
		return fmt.Errorf("kubectl get nodes: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "True") {
			return nil
		}
	}
	return fmt.Errorf("no Ready node; join the VM first (README: Joining a node)")
}

func ensureE2ETest(root string) (string, error) {
	version := strings.SplitN(upstream.Version, "-k3s", 2)[0]
	dir := filepath.Join(root, ".build/e2e", version)
	bin := filepath.Join(dir, "e2e.test")
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://dl.k8s.io/%s/kubernetes-test-%s-%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("downloading %s\n", url)
	tmp, err := downloadTemp(url)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	if err := extractE2ETest(tmp, dir); err != nil {
		return "", err
	}
	return bin, nil
}

func downloadTemp(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.CreateTemp("", "e2e-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func extractE2ETest(tarGzPath, dir string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Base(hdr.Name)
		if hdr.Name != "kubernetes/test/bin/e2e.test" && hdr.Name != "kubernetes/test/bin/ginkgo" {
			continue
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		out.Close()
		if err != nil {
			return err
		}
		if name == "e2e.test" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("kubernetes/test/bin/e2e.test not found in %s", tarGzPath)
	}
	return nil
}

func runE2E(e2eTest, kubeconfig, focus, reportDir string, procs int) error {
	absKubeconfig, err := filepath.Abs(kubeconfig)
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(e2eTest), "ginkgo"),
		"--no-color",
		fmt.Sprintf("--procs=%d", procs),
		"--focus="+focus,
		"--junit-report="+filepath.Join(reportDir, "junit.xml"),
		e2eTest,
		"--",
		"--kubeconfig", absKubeconfig,
		"--provider=skeleton",
		"--num-nodes=1",
		"--disable-log-dump",
		"--report-dir", reportDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

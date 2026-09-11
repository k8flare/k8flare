package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type config struct {
	url     string
	token   string
	name    string
	ns      string
	image   string
	compute string

	runningTimeout time.Duration
	deleteTimeout  time.Duration
	pollInterval   time.Duration

	parking          bool
	parkWait         time.Duration
	nodeDrainTimeout time.Duration
	quietWindow      time.Duration
	analyticsLag     time.Duration

	accountID  string
	apiToken   string
	scriptName string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("prodprobe", flag.ContinueOnError)
	fs.StringVar(&cfg.url, "url", os.Getenv("K8FLARE_PROBE_URL"), "base URL of the deployment to probe, e.g. https://k8flare.example.workers.dev (env K8FLARE_PROBE_URL)")
	fs.StringVar(&cfg.token, "token", os.Getenv("K8FLARE_PROBE_TOKEN"), "cluster token for that deployment (env K8FLARE_PROBE_TOKEN)")
	fs.StringVar(&cfg.name, "name", "k8flare-prodprobe", "name of the Deployment the probe creates and deletes")
	fs.StringVar(&cfg.ns, "namespace", "default", "namespace to run in")
	fs.StringVar(&cfg.image, "image", "registry.k8s.io/pause:3.10", "image for the probe pod")
	fs.StringVar(&cfg.compute, "compute", "containers", "value for the k8flare.com/compute pod annotation; empty runs on whatever nodes are attached")
	fs.DurationVar(&cfg.runningTimeout, "running-timeout", 8*time.Minute, "how long to wait for a pod to reach Running")
	fs.DurationVar(&cfg.deleteTimeout, "delete-timeout", 5*time.Minute, "how long to wait for the pods to disappear after deletion")
	fs.DurationVar(&cfg.pollInterval, "poll-interval", 5*time.Second, "how often to poll while waiting for convergence")
	fs.BoolVar(&cfg.parking, "parking", true, "assert the cluster parks: requires Cloudflare analytics credentials")
	fs.DurationVar(&cfg.parkWait, "park-wait", 6*time.Minute, "how long the cluster is given to quiesce after the workload is gone")
	fs.DurationVar(&cfg.nodeDrainTimeout, "node-drain-timeout", 10*time.Minute, "how long to wait for demand-started nodes to detach before the quiet window")
	fs.DurationVar(&cfg.quietWindow, "quiet-window", 10*time.Minute, "length of the window that must contain zero Worker and Durable Object requests")
	fs.DurationVar(&cfg.analyticsLag, "analytics-lag", 5*time.Minute, "how long to wait for the analytics datasets to catch up before querying")
	fs.StringVar(&cfg.accountID, "account", os.Getenv("CLOUDFLARE_ACCOUNT_ID"), "Cloudflare account id (env CLOUDFLARE_ACCOUNT_ID)")
	fs.StringVar(&cfg.apiToken, "api-token", os.Getenv("CLOUDFLARE_API_TOKEN"), "Cloudflare API token with Account Analytics:Read (env CLOUDFLARE_API_TOKEN)")
	fs.StringVar(&cfg.scriptName, "script", envOr("K8FLARE_PROBE_SCRIPT", "k8flare"), "Worker script name as it appears in analytics (env K8FLARE_PROBE_SCRIPT)")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.url == "" {
		return cfg, fmt.Errorf("-url is required")
	}
	cfg.url = strings.TrimSuffix(cfg.url, "/")
	if cfg.token == "" {
		return cfg, fmt.Errorf("-token is required")
	}
	if cfg.parking && (cfg.accountID == "" || cfg.apiToken == "") {
		return cfg, fmt.Errorf("-parking needs -account and -api-token; pass -parking=false to run the convergence phases alone")
	}
	return cfg, nil
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "prodprobe: %v\n", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "prodprobe: FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "prodprobe: PASS")
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	step := func(format string, args ...any) {
		fmt.Fprintf(out, "%s  %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
	}

	step("probing %s (script %q)", cfg.url, cfg.scriptName)
	if err := checkReadyz(ctx, cfg, out); err != nil {
		return err
	}

	cs, err := newClientset(cfg.url, cfg.token)
	if err != nil {
		return fmt.Errorf("build kubernetes client: %w", err)
	}
	selector := "app=" + cfg.name

	if err := deleteDeployment(ctx, cs, cfg.ns, cfg.name); err != nil {
		return fmt.Errorf("clear a previous run's deployment: %w", err)
	}
	if err := waitForPodsGone(ctx, cs, cfg.ns, selector, cfg.deleteTimeout, cfg.pollInterval); err != nil {
		return fmt.Errorf("clear a previous run's pods: %w", err)
	}

	activeStart := time.Now()
	step("creating deployment %s/%s (image %s, compute %q)", cfg.ns, cfg.name, cfg.image, cfg.compute)
	if _, err := cs.AppsV1().Deployments(cfg.ns).Create(ctx, probeDeployment(cfg.name, cfg.image, cfg.compute), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if err := deleteDeployment(cleanup, cs, cfg.ns, cfg.name); err != nil {
			fmt.Fprintf(out, "prodprobe: cleanup of %s/%s failed: %v\n", cfg.ns, cfg.name, err)
		}
	}()

	pods, err := waitForRunningPods(ctx, cs, cfg.ns, selector, 1, cfg.runningTimeout, cfg.pollInterval)
	if err != nil {
		return err
	}
	step("pod %s reached Running after %s", pods[0], time.Since(activeStart).Round(time.Second))

	scaledAt := time.Now()
	step("scaling %s/%s to 2", cfg.ns, cfg.name)
	if err := scaleDeployment(ctx, cs, cfg.ns, cfg.name, 2); err != nil {
		return fmt.Errorf("scale deployment: %w", err)
	}
	pods, err = waitForRunningPods(ctx, cs, cfg.ns, selector, 2, cfg.runningTimeout, cfg.pollInterval)
	if err != nil {
		return fmt.Errorf("a scale-up after the first convergence was not acted on: %w", err)
	}
	step("both pods Running %s after the scale (%s)", time.Since(scaledAt).Round(time.Second), strings.Join(pods, ", "))

	step("deleting deployment %s/%s", cfg.ns, cfg.name)
	if err := deleteDeployment(ctx, cs, cfg.ns, cfg.name); err != nil {
		return fmt.Errorf("delete deployment: %w", err)
	}
	if err := waitForPodsGone(ctx, cs, cfg.ns, selector, cfg.deleteTimeout, cfg.pollInterval); err != nil {
		return err
	}
	activeEnd := time.Now()
	step("workload gone after %s total", activeEnd.Sub(activeStart).Round(time.Second))

	if !cfg.parking {
		step("PARKING ASSERTION DISABLED (-parking=false): convergence passed, idle cost unverified")
		return nil
	}
	return assertParked(ctx, cfg, cs, activeStart, activeEnd, step)
}

func checkReadyz(ctx context.Context, cfg config, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.url+"/readyz?verbose=true", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.token)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("GET /readyz: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Fprintf(out, "%s", body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /readyz: HTTP %d", resp.StatusCode)
	}
	return nil
}

// awaitNodesDrained polls until no node is attached, or the timeout expires.
// Returns the nodes still attached, so the caller reports them rather than a
// timeout error: a node that never leaves is a real finding, not a probe fault.
func awaitNodesDrained(ctx context.Context, cfg config, cs kubernetes.Interface, step func(string, ...any)) ([]string, error) {
	deadline := time.Now().Add(cfg.nodeDrainTimeout)
	for {
		nodes, err := countNodes(ctx, cs)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		if len(nodes) == 0 || time.Now().After(deadline) {
			return nodes, nil
		}
		step("waiting for %d demand-started node(s) to detach (%s)", len(nodes), strings.Join(nodes, ", "))
		select {
		case <-ctx.Done():
			return nodes, ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

func assertParked(ctx context.Context, cfg config, cs kubernetes.Interface, activeStart, activeEnd time.Time, step func(string, ...any)) error {
	// A demand-started NodeVM outlives the workload that caused it: measured
	// 2026-09-11 against production, the two nodes this probe's own pods ran
	// on were still attached when the deployment was already gone, and had
	// detached about two minutes later. Failing on that first reading blamed
	// the control plane for the probe being early, so wait for them first.
	nodes, err := awaitNodesDrained(ctx, cfg, cs, step)
	if err != nil {
		return err
	}
	if len(nodes) > 0 {
		return fmt.Errorf("cannot assert idle cost with %d node(s) still attached after %s (%s): a kubelet heartbeats every ~10s, so the control plane is never quiet. Detach the nodes, use -compute containers so the node is demand-started, or pass -parking=false and say so",
			len(nodes), cfg.nodeDrainTimeout, strings.Join(nodes, ", "))
	}

	sleepStep(ctx, cfg.parkWait, "letting the cluster quiesce", step)
	quietStart := time.Now()
	sleepStep(ctx, cfg.quietWindow, "measuring the quiet window", step)
	quietEnd := time.Now()
	sleepStep(ctx, cfg.analyticsLag, "waiting for the analytics datasets to catch up", step)

	client := newAnalyticsClient(cfg.accountID, cfg.apiToken, cfg.scriptName)

	control, err := client.invocations(ctx, activeStart, activeEnd)
	if err != nil {
		return fmt.Errorf("instrument check: querying the active window failed: %w", err)
	}
	step("active window %s..%s: %s", stamp(activeStart), stamp(activeEnd), control)

	quiet, err := client.invocations(ctx, quietStart, quietEnd)
	if err != nil {
		return fmt.Errorf("querying the quiet window failed: %w", err)
	}
	step("quiet window %s..%s: %s", stamp(quietStart), stamp(quietEnd), quiet)

	return evaluateParking(control, quiet, cfg.scriptName, cfg.quietWindow)
}

func evaluateParking(control, quiet invocations, scriptName string, quietWindow time.Duration) error {
	if control.total() == 0 {
		return fmt.Errorf("instrument check: the active window reports zero requests, but the probe itself drove a Deployment through it. The analytics query is measuring nothing -- wrong -script (%q), wrong -account, or the query shape in analytics.go no longer matches the datasets. Refusing to report a green idle window measured by a broken instrument",
			scriptName)
	}
	if quiet.total() != 0 {
		return fmt.Errorf("the cluster did not park: %s in the %s after the workload was deleted (cost invariants #1/#3). Expected zero of both",
			quiet, quietWindow)
	}
	return nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func sleepStep(ctx context.Context, d time.Duration, what string, step func(string, ...any)) {
	step("%s: %s", what, d)
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		chunk := time.Minute
		if remaining < chunk {
			chunk = remaining
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(chunk):
		}
		if r := time.Until(deadline); r > 0 {
			step("  %s remaining", r.Round(time.Second))
		}
	}
}

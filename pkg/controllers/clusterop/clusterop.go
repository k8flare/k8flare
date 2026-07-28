//go:build js && wasm

// Package clusterop hosts the cluster operator: the controller that turns
// k8flare.com/v1alpha1 Cluster objects in the management ("default")
// cluster into real tenant control planes, and back. It is the fourth
// resident dynamic worker (pkg/controllers/cmd/clusterop-wasm), poked by
// the Controllers DO exactly like kcm/gc/sched.
//
// Its own package, not pkg/controllers or pkg/controllers/gc: every
// *-wasm entrypoint that shares a package links that package's other
// controllers' reachable code whether or not it calls them -- measured at
// ~4MB when pkg/controllers/gc briefly lived in pkg/controllers (see
// pkg/controllers/restconfig's doc comment).
//
// Unlike every other controller this repo runs, there is no upstream
// implementation to embed here: k8flare's own management API has no
// counterpart in kubernetes/kubernetes. What IS reused is all the
// machinery around the reconcile -- cache.SharedIndexInformer,
// cache.ListWatch, and client-go's typed workqueue with its rate limiter
// -- so the hand-written part is the reconcile body alone.
package clusterop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apimachinerywatch "k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	k8flarev1alpha1 "github.com/k8flare/k8flare/pkg/apis/k8flare/v1alpha1"
	leancorev1 "github.com/k8flare/k8flare/pkg/leanclient/gen/corev1"
)

const (
	// TeardownFinalizer holds a Cluster object alive until its Durable
	// Object tree, Containers, and registry entry are actually gone. The
	// cascade is a procedural teardown, not ownerReferences GC -- DOs and
	// Containers are not Kubernetes objects, so the real garbage collector
	// has no jurisdiction (docs/cluster-api-design.md, review point #7).
	TeardownFinalizer = "k8flare.com/cluster-teardown"

	// RotateAnnotation requests a token rotation. Its VALUE is arbitrary
	// and only has to change; the operator mints a fresh token, republishes
	// the Secret, revokes the superseded ones, and clears the annotation.
	RotateAnnotation = "k8flare.com/rotate-token"

	// SecretNamespace is where every cluster's distribution Secret lives.
	// The tenant's own Cluster DO vault stays authoritative; these Secrets
	// are mirrors, which is why losing one is repairable (the operator
	// rewrites it from the vault) and why they are deliberately NOT in
	// storage's CONTROLLER_RELEVANT_PREFIXES -- a Secret write must not
	// poke the controllers pump and re-drive this reconcile.
	SecretNamespace = "k8flare-system"

	// DefaultClusterName is the management cluster's own Cluster object.
	// Its Durable Object tree predates the object and is literally named
	// "default" (no uid suffix), and it has no registry record -- the
	// gateway resolves un-prefixed paths to it without consulting one.
	DefaultClusterName = "default"

	// ConditionRotateUnsupported records a rotation request that was
	// refused permanently rather than retried (see rejectRotation).
	ConditionRotateUnsupported = "RotateUnsupported"

	// ConditionReconcileError carries the last reconcile failure onto the
	// object itself, so a stalled cluster is diagnosable with kubectl alone
	// (`kubectl get cluster <name> -o yaml`) instead of requiring a live
	// tail of the dynamic worker.
	ConditionReconcileError = "ReconcileError"

	// maxConditionMessage bounds what a bridge error (which embeds the
	// shell Worker's whole response body) can write into etcd/kine.
	maxConditionMessage = 256

	phaseProvisioning = "Provisioning"
	phaseReady        = "Ready"
	phaseTerminating  = "Terminating"
)

// logf writes one console-visible line. Go's stdout/stderr writes reach
// the dynamic worker's console.log through wasm_exec.js's globalThis.fs
// shim (it line-buffers and console.logs on each "\n"), which is what
// makes these show up in `wrangler tail` -- verified 2026-07-28, when the
// operator's total silence during the intermittent production reconcile
// stall turned out to be "it never logged", not "its logs are dropped"
// (docs/cluster-api-design.md).
func logf(format string, args ...interface{}) {
	fmt.Printf("cluster-operator: "+format+"\n", args...)
}

// buildTag identifies which build is running, so a tail line can be
// attributed to a deployment rather than guessed at.
func buildTag() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	rev := "norev"
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			rev = s.Value
		}
	}
	return info.GoVersion + "/" + rev
}

type controller struct {
	clusters *clustersClient
	core     *leancorev1.Client
	bridge   *bridge
	queue    workqueue.TypedRateLimitingInterface[string]
	informer cache.SharedIndexInformer

	// failing tracks which keys currently carry a ConditionReconcileError,
	// so the recovery path can clear it without a Get on every successful
	// reconcile. Only ever touched from the single processNext loop.
	failing map[string]bool
}

// Run starts the cluster operator against restCfg (the management
// cluster's API) and blocks until ctx is canceled. bindingName is the
// service binding the platform-operations bridge speaks over, and token
// authenticates both.
func Run(ctx context.Context, restCfg *restclient.Config, bindingName, token string) (err error) {
	logf("starting (build %s)", buildTag())
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cluster-operator: panic: %v", r)
			logf("panic: %v", r)
			logf("%s", debug.Stack())
		}
		logf("Run returning: %v", err)
	}()

	clusters, err := newClustersClient(restclient.AddUserAgent(restCfg, "cluster-operator"))
	if err != nil {
		return fmt.Errorf("cluster-operator: build clusters client: %w", err)
	}
	core, err := leancorev1.New(restclient.AddUserAgent(restCfg, "cluster-operator"))
	if err != nil {
		return fmt.Errorf("cluster-operator: build core client: %w", err)
	}

	c := &controller{
		clusters: clusters,
		core:     core,
		bridge:   newBridge(bindingName, token),
		queue: workqueue.NewTypedRateLimitingQueue[string](
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		failing: map[string]bool{},
	}

	// Same survival posture as pkg/controllers/gc: a panic inside the
	// informer/reflector goroutine tree must not take the whole resident
	// instance down (which would just reload-loop with no visible cause).
	utilruntime.ReallyCrash = false
	utilruntime.PanicHandlers = append(utilruntime.PanicHandlers, func(_ context.Context, r interface{}) {
		logf("captured panic: %v", r)
		logf("%s", debug.Stack())
	})

	c.informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				list, err := c.clusters.List(context.Background(), opts)
				if err != nil {
					logf("informer LIST failed: %v", err)
					return nil, err
				}
				logf("informer LIST ok: %d clusters at rv=%s", len(list.Items), list.ResourceVersion)
				return list, nil
			},
			WatchFunc: func(opts metav1.ListOptions) (apimachinerywatch.Interface, error) {
				w, err := c.clusters.Watch(context.Background(), opts)
				if err != nil {
					logf("informer WATCH from rv=%s failed: %v", opts.ResourceVersion, err)
					return nil, err
				}
				logf("informer WATCH established from rv=%s", opts.ResourceVersion)
				return w, nil
			},
		},
		&k8flarev1alpha1.Cluster{},
		0, // no resync: watch events drive this, like every other controller here
		cache.Indexers{},
	)
	if _, err := c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.enqueue(obj) },
		UpdateFunc: func(_, obj interface{}) { c.enqueue(obj) },
		DeleteFunc: func(obj interface{}) { c.enqueue(obj) },
	}); err != nil {
		return fmt.Errorf("cluster-operator: add event handler: %w", err)
	}

	go c.informer.Run(ctx.Done())
	logf("waiting for informer cache sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		logf("cache sync failed (ctx err: %v)", ctx.Err())
		return fmt.Errorf("cluster-operator: cache sync failed")
	}
	logf("informer cache synced")

	// Startup work, in this order: seed the management cluster's own
	// Cluster object so `kubectl get clusters` shows the whole truth, then
	// rebuild the resolution cache from the Cluster list. The registry is
	// a pure cache (docs/cluster-api-design.md review point #5) -- if it
	// were lost entirely, this is what puts it back.
	c.seedDefaultCluster(ctx)
	c.rebuildRegistry(ctx)

	defer c.queue.ShutDown()
	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()
	logf("startup complete, entering work loop")
	for c.processNext(ctx) {
	}
	logf("work loop ended (queue shut down); ctx err: %v", ctx.Err())
	return ctx.Err()
}

func (c *controller) enqueue(obj interface{}) {
	if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
		c.queue.Add(key)
	}
}

func (c *controller) processNext(ctx context.Context) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	attempt := c.queue.NumRequeues(key) + 1
	start := time.Now()
	logf("reconcile %s: begin (attempt %d, queue depth %d)", key, attempt, c.queue.Len())
	err := c.reconcile(ctx, key)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		logf("reconcile %s: FAILED after %s (attempt %d): %v", key, elapsed, attempt, err)
		c.recordReconcileError(ctx, key, err)
		c.queue.AddRateLimited(key)
		logf("reconcile %s: requeued rate-limited (requeues now %d)", key, c.queue.NumRequeues(key))
		return true
	}
	logf("reconcile %s: ok in %s", key, elapsed)
	c.clearReconcileError(ctx, key)
	c.queue.Forget(key)
	return true
}

// recordReconcileError stamps the failure onto the object's status so a
// stall is diagnosable with `kubectl get cluster -o yaml` and not only
// from a live tail. Best-effort: this runs on a path that is ALREADY
// failing, and its own failure must not replace the real error.
func (c *controller) recordReconcileError(ctx context.Context, name string, cause error) {
	msg := cause.Error()
	if len(msg) > maxConditionMessage {
		msg = msg[:maxConditionMessage] + "..."
	}
	cl, err := c.clusters.Get(ctx, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logf("reconcile %s: could not read object to record error condition: %v", name, err)
		}
		return
	}
	status := *cl.Status.DeepCopy()
	apimeta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               ConditionReconcileError,
		Status:             metav1.ConditionTrue,
		Reason:             "ReconcileFailed",
		Message:            msg,
		ObservedGeneration: cl.Generation,
	})
	if err := c.updateStatus(ctx, cl, status); err != nil {
		logf("reconcile %s: could not write error condition: %v", name, err)
		return
	}
	c.failing[name] = true
}

// clearReconcileError removes the condition a previous failure left
// behind. Gated on c.failing so a steady-state reconcile costs no extra
// API call -- and so it never writes to an object it did not mark.
func (c *controller) clearReconcileError(ctx context.Context, name string) {
	if !c.failing[name] {
		return
	}
	delete(c.failing, name)
	cl, err := c.clusters.Get(ctx, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logf("reconcile %s: could not read object to clear error condition: %v", name, err)
		}
		return
	}
	status := *cl.Status.DeepCopy()
	if !apimeta.RemoveStatusCondition(&status.Conditions, ConditionReconcileError) {
		return
	}
	if err := c.updateStatus(ctx, cl, status); err != nil {
		logf("reconcile %s: could not clear error condition: %v", name, err)
	}
}

// seedDefaultCluster makes the management cluster visible as a Cluster
// object named "default". Creating it is best-effort and an AlreadyExists
// is the normal steady-state answer, not an error.
func (c *controller) seedDefaultCluster(ctx context.Context) {
	_, err := c.clusters.Create(ctx, &k8flarev1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultClusterName},
		Spec:       k8flarev1alpha1.ClusterSpec{DisplayName: "default"},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logf("seed default cluster failed: %v", err)
		return
	}
	if err == nil {
		logf("seeded the default Cluster object")
	}
}

// rebuildRegistry re-derives every resolution-cache entry from the
// Cluster objects, which are the truth. Only clusters that already have a
// doName are replayed: one without a doName has not been provisioned yet,
// and its own reconcile will write the entry.
func (c *controller) rebuildRegistry(ctx context.Context) {
	list, err := c.clusters.List(ctx, metav1.ListOptions{})
	if err != nil {
		logf("registry rebuild LIST failed: %v", err)
		return
	}
	replayed := 0
	for i := range list.Items {
		cl := &list.Items[i]
		if cl.Name == DefaultClusterName || cl.DeletionTimestamp != nil {
			continue
		}
		uid := uidFromDoName(cl.Status.DoName)
		if uid == "" {
			continue
		}
		if err := c.bridge.UpsertRegistry(ctx, cl.Name, uid); err != nil {
			logf("registry rebuild %s failed: %v", cl.Name, err)
			continue
		}
		replayed++
	}
	logf("registry rebuild: replayed %d of %d clusters", replayed, len(list.Items))
}

func uidFromDoName(doName string) string {
	at := strings.LastIndex(doName, "@")
	if at < 0 {
		return ""
	}
	return doName[at+1:]
}

func (c *controller) reconcile(ctx context.Context, name string) error {
	cl, err := c.clusters.Get(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // gone, and its finalizer already ran
		}
		return err
	}
	if cl.DeletionTimestamp != nil {
		return c.reconcileDelete(ctx, cl)
	}
	return c.reconcileActive(ctx, cl)
}

// converged answers the poke-feedback guard: is there any reason to touch
// this object again? The operator's own status write bumps the object's
// resourceVersion and comes back through the informer, so without this a
// steady-state cluster would reconcile forever (docs/cluster-api-design.md's
// "poke feedback prevention"). Exported-shaped as a free function so it
// can be unit-tested without a live API.
func converged(cl *k8flarev1alpha1.Cluster) bool {
	if cl.DeletionTimestamp != nil {
		return false
	}
	if cl.Annotations[RotateAnnotation] != "" {
		return false
	}
	if cl.Status.Phase != phaseReady {
		return false
	}
	if cl.Status.ObservedGeneration != cl.Generation {
		return false
	}
	return hasFinalizer(cl)
}

func hasFinalizer(cl *k8flarev1alpha1.Cluster) bool {
	for _, f := range cl.Finalizers {
		if f == TeardownFinalizer {
			return true
		}
	}
	return false
}

// doNameFor allocates this cluster's Durable Object tree name. The
// operator is the ONLY allocator of doNames (docs/cluster-api-design.md
// review point #4) and the object's own uid is the discriminator, so a
// re-created cluster of the same name never inherits the deleted one's DO
// state. Already-allocated names are kept verbatim.
func doNameFor(cl *k8flarev1alpha1.Cluster) string {
	if cl.Name == DefaultClusterName {
		return DefaultClusterName
	}
	if cl.Status.DoName != "" {
		return cl.Status.DoName
	}
	return fmt.Sprintf("%s@%s", cl.Name, cl.UID)
}

func (c *controller) reconcileActive(ctx context.Context, cl *k8flarev1alpha1.Cluster) error {
	if !hasFinalizer(cl) {
		updated := cl.DeepCopy()
		updated.Finalizers = append(updated.Finalizers, TeardownFinalizer)
		if _, err := c.clusters.Update(ctx, updated); err != nil {
			return fmt.Errorf("add finalizer: %w", err)
		}
		logf("reconcile %s: added teardown finalizer", cl.Name)
		return nil // the update re-enqueues this object
	}
	if converged(cl) {
		logf("reconcile %s: converged (phase=%s generation=%d), nothing to do", cl.Name, cl.Status.Phase, cl.Generation)
		return nil
	}
	logf("reconcile %s: active pass (phase=%q generation=%d observed=%d rotate=%t)",
		cl.Name, cl.Status.Phase, cl.Generation, cl.Status.ObservedGeneration,
		cl.Annotations[RotateAnnotation] != "")

	doName := doNameFor(cl)
	rotate := cl.Annotations[RotateAnnotation]

	// The management cluster's credential is the K3S_TOKEN root secret
	// rather than a vault entry (see internalapi.ts's handleVault), so
	// there is nothing here to rotate and the bridge answers 409. That is
	// a permanent answer, not a transient one: retrying it would just
	// rate-limit-loop forever. Treat it as terminal -- say so in a
	// condition and drop the annotation.
	if rotate != "" && cl.Name == DefaultClusterName {
		return c.rejectRotation(ctx, cl)
	}

	// Resolution cache first: an entry without credentials 401s, which the
	// next few lines fix; credentials without an entry 404 forever.
	if cl.Name != DefaultClusterName {
		if err := c.bridge.UpsertRegistry(ctx, cl.Name, uidFromDoName(doName)); err != nil {
			return fmt.Errorf("upsert registry: %w", err)
		}
	}

	var vault *vaultResult
	var err error
	if rotate != "" {
		vault, err = c.bridge.MintToken(ctx, doName, rotationTokenID(rotate))
	} else {
		vault, err = c.bridge.EnsureVault(ctx, doName)
	}
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	logf("reconcile %s: vault ok (doName=%s tokenId=%s superseded=%d)", cl.Name, doName, vault.TokenID, len(vault.Superseded))

	// An empty token means this cluster's credential is not vault-managed:
	// the management cluster authenticates with the K3S_TOKEN root secret,
	// which the operator neither mints nor mirrors (see internalapi.ts's
	// handleVault -- minting one would invalidate that root token).
	var secretRef *k8flarev1alpha1.SecretReference
	if vault.Token != "" {
		if err := c.ensureSecret(ctx, cl, vault); err != nil {
			return err
		}
		secretRef = &k8flarev1alpha1.SecretReference{
			Namespace: SecretNamespace,
			Name:      secretName(cl.Name),
		}
	}

	// Only now, with the replacement distributed, is it safe to revoke
	// what it superseded.
	for _, id := range vault.Superseded {
		if err := c.bridge.RevokeToken(ctx, doName, id); err != nil {
			return fmt.Errorf("revoke superseded token %s: %w", id, err)
		}
	}
	if rotate != "" {
		updated := cl.DeepCopy()
		delete(updated.Annotations, RotateAnnotation)
		if _, err := c.clusters.Update(ctx, updated); err != nil {
			return fmt.Errorf("clear rotate annotation: %w", err)
		}
		return nil // the update re-enqueues; status settles on that pass
	}

	return c.updateStatus(ctx, cl, k8flarev1alpha1.ClusterStatus{
		Phase:              phaseReady,
		DoName:             doName,
		Endpoint:           vault.Endpoint,
		ObservedGeneration: cl.Generation,
		TokenSecretRef:     secretRef,
		// Carried over, not rebuilt: conditions are set by the paths that
		// own them (rejectRotation), and dropping them here would erase
		// that record on the very next pass.
		Conditions: cl.Status.Conditions,
	})
}

func (c *controller) reconcileDelete(ctx context.Context, cl *k8flarev1alpha1.Cluster) error {
	if !hasFinalizer(cl) {
		return nil
	}
	logf("reconcile %s: delete pass (phase=%q)", cl.Name, cl.Status.Phase)
	if cl.Status.Phase != phaseTerminating {
		status := *cl.Status.DeepCopy()
		status.Phase = phaseTerminating
		if err := c.updateStatus(ctx, cl, status); err != nil {
			return err
		}
		// Fall through rather than returning: the status write re-enqueues,
		// but the teardown below is idempotent and there is no reason to
		// wait a round trip before starting it.
	}
	doName := doNameFor(cl)
	if cl.Name != DefaultClusterName {
		if err := c.bridge.Teardown(ctx, cl.Name, doName); err != nil {
			return fmt.Errorf("teardown: %w", err)
		}
	}
	if err := c.core.Secrets(SecretNamespace).Delete(ctx, secretName(cl.Name), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete secret: %w", err)
	}

	fresh, err := c.clusters.Get(ctx, cl.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	updated := fresh.DeepCopy()
	kept := updated.Finalizers[:0]
	for _, f := range updated.Finalizers {
		if f != TeardownFinalizer {
			kept = append(kept, f)
		}
	}
	updated.Finalizers = kept
	if _, err := c.clusters.Update(ctx, updated); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("remove finalizer: %w", err)
	}
	logf("reconcile %s: teardown complete, finalizer removed", cl.Name)
	return nil
}

func secretName(cluster string) string { return "cluster-" + cluster }

// rotationTokenID derives the vault token id a rotation mints under, from
// the rotate annotation's value alone. Deterministic on purpose: it is
// what makes the whole rotation replayable. A reconcile can fail after
// minting but before rewriting the Secret, revoking the superseded
// tokens, or clearing the annotation -- and the retry then re-requests
// this same id, gets the token it already minted back, and finishes the
// remaining steps. With a random id per attempt, every failed attempt
// would instead strand another permanently-valid token in the vault.
func rotationTokenID(annotation string) string {
	sum := sha256.Sum256([]byte(annotation))
	return hex.EncodeToString(sum[:])[:8]
}

// rejectRotation answers a rotation request that can never succeed (the
// management cluster's, see reconcileActive) by clearing the annotation
// and recording why. Terminal: nothing re-drives it.
func (c *controller) rejectRotation(ctx context.Context, cl *k8flarev1alpha1.Cluster) error {
	updated := cl.DeepCopy()
	delete(updated.Annotations, RotateAnnotation)
	fresh, err := c.clusters.Update(ctx, updated)
	if err != nil {
		return fmt.Errorf("clear unsupported rotate annotation: %w", err)
	}
	status := *fresh.Status.DeepCopy()
	apimeta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               ConditionRotateUnsupported,
		Status:             metav1.ConditionTrue,
		Reason:             "ManagementClusterToken",
		Message:            "the default cluster's credential is the K3S_TOKEN secret and is not rotatable through the Cluster API",
		ObservedGeneration: fresh.Generation,
	})
	return c.updateStatus(ctx, fresh, status)
}

// ensureSecret publishes the cluster's credentials into
// k8flare-system/cluster-<name>. The namespace is created first and its
// AlreadyExists tolerated: the apiserver's NamespaceLifecycle admission
// rejects creates into a namespace that does not exist, so the ordering
// is load-bearing, not defensive.
func (c *controller) ensureSecret(ctx context.Context, cl *k8flarev1alpha1.Cluster, vault *vaultResult) error {
	if _, err := c.core.Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: SecretNamespace},
	}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create %s namespace: %w", SecretNamespace, err)
	}

	name := secretName(cl.Name)
	data := map[string][]byte{
		"token":      []byte(vault.Token),
		"kubeconfig": []byte(vault.Kubeconfig),
	}
	existing, err := c.core.Secrets(SecretNamespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.core.Secrets(SecretNamespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: SecretNamespace,
				Labels:    map[string]string{"k8flare.com/cluster": cl.Name},
			},
			Type: corev1.SecretTypeOpaque,
			Data: data,
		}, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create secret: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}
	if secretDataEqual(existing.Data, data) {
		return nil // no diff: writing anyway would just churn resourceVersions
	}
	updated := existing.DeepCopy()
	updated.Data = data
	if _, err := c.core.Secrets(SecretNamespace).Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret: %w", err)
	}
	return nil
}

func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || string(av) != string(bv) {
			return false
		}
	}
	return true
}

// updateStatus writes status only when it actually differs. Every status
// write is an object write that comes back through the informer and, via
// storage's pingControllers, re-pokes this very controller -- so a
// write-unconditionally version is an infinite reconcile loop, not merely
// wasteful.
func (c *controller) updateStatus(ctx context.Context, cl *k8flarev1alpha1.Cluster, want k8flarev1alpha1.ClusterStatus) error {
	if statusEqual(cl.Status, want) {
		return nil
	}
	updated := cl.DeepCopy()
	updated.Status = want
	if _, err := c.clusters.UpdateStatus(ctx, updated); err != nil {
		if apierrors.IsConflict(err) {
			// Worth naming separately: a conflict means someone else wrote
			// the object underneath this pass, and the retry is expected to
			// succeed -- unlike the other failures here.
			logf("reconcile %s: status update conflict (rv=%s), will retry", cl.Name, cl.ResourceVersion)
		}
		return fmt.Errorf("update status: %w", err)
	}
	logf("reconcile %s: status written (phase=%s observed=%d)", cl.Name, want.Phase, want.ObservedGeneration)
	return nil
}

func statusEqual(a, b k8flarev1alpha1.ClusterStatus) bool {
	if a.Phase != b.Phase || a.DoName != b.DoName || a.Endpoint != b.Endpoint ||
		a.ObservedGeneration != b.ObservedGeneration {
		return false
	}
	if (a.TokenSecretRef == nil) != (b.TokenSecretRef == nil) {
		return false
	}
	if a.TokenSecretRef != nil && *a.TokenSecretRef != *b.TokenSecretRef {
		return false
	}
	// Conditions count too, or a status whose ONLY change is a condition
	// (rejectRotation's) would be silently dropped. LastTransitionTime is
	// deliberately excluded: apimeta.SetStatusCondition only moves it when
	// the status field itself changes, so comparing the meaningful fields
	// keeps this from churning resourceVersions on every pass.
	if len(a.Conditions) != len(b.Conditions) {
		return false
	}
	for i := range a.Conditions {
		x, y := a.Conditions[i], b.Conditions[i]
		if x.Type != y.Type || x.Status != y.Status || x.Reason != y.Reason ||
			x.Message != y.Message || x.ObservedGeneration != y.ObservedGeneration {
			return false
		}
	}
	return true
}

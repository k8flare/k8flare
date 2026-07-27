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
	"fmt"
	"runtime/debug"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	phaseProvisioning = "Provisioning"
	phaseReady        = "Ready"
	phaseTerminating  = "Terminating"
)

type controller struct {
	clusters *clustersClient
	core     *leancorev1.Client
	bridge   *bridge
	queue    workqueue.TypedRateLimitingInterface[string]
	informer cache.SharedIndexInformer
}

// Run starts the cluster operator against restCfg (the management
// cluster's API) and blocks until ctx is canceled. bindingName is the
// service binding the platform-operations bridge speaks over, and token
// authenticates both.
func Run(ctx context.Context, restCfg *restclient.Config, bindingName, token string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cluster-operator: panic: %v", r)
		}
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
	}

	// Same survival posture as pkg/controllers/gc: a panic inside the
	// informer/reflector goroutine tree must not take the whole resident
	// instance down (which would just reload-loop with no visible cause).
	utilruntime.ReallyCrash = false
	utilruntime.PanicHandlers = append(utilruntime.PanicHandlers, func(_ context.Context, r interface{}) {
		println("cluster-operator: captured panic:", fmt.Sprint(r))
		println(string(debug.Stack()))
	})

	c.informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				return c.clusters.List(context.Background(), opts)
			},
			WatchFunc: func(opts metav1.ListOptions) (apimachinerywatch.Interface, error) {
				return c.clusters.Watch(context.Background(), opts)
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
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return fmt.Errorf("cluster-operator: cache sync failed")
	}

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
	for c.processNext(ctx) {
	}
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
	if err := c.reconcile(ctx, key); err != nil {
		println("cluster-operator: reconcile", key, "failed:", err.Error())
		c.queue.AddRateLimited(key)
		return true
	}
	c.queue.Forget(key)
	return true
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
		println("cluster-operator: seed default cluster:", err.Error())
	}
}

// rebuildRegistry re-derives every resolution-cache entry from the
// Cluster objects, which are the truth. Only clusters that already have a
// doName are replayed: one without a doName has not been provisioned yet,
// and its own reconcile will write the entry.
func (c *controller) rebuildRegistry(ctx context.Context) {
	list, err := c.clusters.List(ctx, metav1.ListOptions{})
	if err != nil {
		println("cluster-operator: registry rebuild list failed:", err.Error())
		return
	}
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
			println("cluster-operator: registry rebuild", cl.Name, "failed:", err.Error())
		}
	}
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
		return nil // the update re-enqueues this object
	}
	if converged(cl) {
		return nil
	}

	doName := doNameFor(cl)
	rotate := cl.Annotations[RotateAnnotation]

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
		vault, err = c.bridge.MintToken(ctx, doName)
	} else {
		vault, err = c.bridge.EnsureVault(ctx, doName)
	}
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}

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
	})
}

func (c *controller) reconcileDelete(ctx context.Context, cl *k8flarev1alpha1.Cluster) error {
	if !hasFinalizer(cl) {
		return nil
	}
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
	return nil
}

func secretName(cluster string) string { return "cluster-" + cluster }

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
		return fmt.Errorf("update status: %w", err)
	}
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
	return true
}

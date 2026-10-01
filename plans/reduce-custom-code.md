# Reducing version-bound code

Goal, from the owner (2026-10-02): less code that is tied to one upstream
version, and generation from upstream source by AST analysis in place of
hand-maintained copies, so that a Kubernetes or k3s bump is cheap. Work on
an area starts once CI covers it.

Status: proposal, not approved. Nothing below is implemented.

## Where the code is today

Measured on 2ad06ad. Go under `packages/`, tests excluded.

| Kind | Lines | What it is |
|---|---|---|
| Generated (`zz_generated_*`) | 35,614 | resources, printers, OpenAPI |
| Own code | 18,860 | bridge, loader, queues, edge routing, supervisor glue |
| Reimplements upstream | 14,036 | admission plugins, authorizer, controllers, registry pieces |
| Wraps upstream | 3,892 | scheduler, HPA, attach/detach, most controllers |
| Tables that copy an upstream list | 539 | plus the TypeScript prefix tables and Makefile lists |

The mirror layer (`scripts/mirror`): 8 upstream modules, 23 overlay files
(1,043 lines), 43 exact-text patches in `main.go`, 43 sha256 pins on the
upstream files the overlays replace.

Only `genprinters` and `genopenapi` use `go/ast` today. Nothing uses
`go/types`.

What a bump costs today: any change to a pinned upstream file stops
`make mirrors`; any reformatting of a patched line stops it; the lean
clientset and informer factory are edited by hand in several places. The
weekly bump workflow that existed before the rewrite was not carried over
(issue #2 is its last failure).

Two failures of 2026-10-01 came from this layer, not from logic:

- `clientset.go` in the overlay is a hand-narrowed copy of upstream's. One
  constructor was missing a group that the other had, and the first pass
  that used it panicked (0114228).
- `WORKLOAD_PREFIXES` in `cluster.ts` and `sources`/`controllerNeeds` in
  `workloads.go` are separate hand lists of which resource wakes which
  controller. Three controllers were linked and never woken (252dd21).

## What changes, in the order proposed

Each step keeps behaviour identical and is checked the same way: the
generated output is first made byte-identical to what is hand-written
today, the hand-written file is deleted in the same commit, then Unit, E2E
and three Conformance runs.

### 1. Mirror transforms addressed by declaration, not by text

- The lean clientset, the informer factory and the nine group `interface.go`
  files become output of a generator that parses upstream's file and keeps
  the group-versions in a list. The list itself comes from `go/types`: the
  group clients and informers that the js build of `packages/` actually
  calls. No hand list, and both constructors come from the same pass.
- The 43 text patches become AST edits that name what they change: "in
  package P, function F, replace the call to X", "drop import I and
  replace its uses with a literal", "give this file a `!js` tag". An edit
  whose target is gone fails with the name of the declaration. This is
  what the sha256 pins are for today, so the pins go away with it.
- Stubs that replace a heavy package (etcd factory, tracing, CEL parser,
  P&F filter) are generated from the upstream package's exported
  signatures: same names and types, bodies that return the zero value or a
  fixed error. The handful with real bodies stay as overlays.
- Stays hand-written: `remotedialer.ServeConn` and the two-pass session
  close, the k3s kubeconfig and tunnel hooks, `hubGroupVersionFor`, the
  webhook transport hook, the workqueue delay observer. These are behaviour
  this project adds. They move to AST edits that insert a named
  declaration, so they no longer depend on surrounding text.

Removes about 800 of 1,043 overlay lines, all 43 pins and the text patches.

### 2. Tables generated from what the code references

- `sources` and `controllerNeeds` (`workloads.go`), and the prefix tables in
  `cluster.ts`, are derived from one fact: which informers each controller
  constructor in `workloads/shards/*` is handed. `go/types` reads that from
  the shard source. The generator writes the Go table and a
  `zz_generated` TypeScript module that `cluster.ts` imports.
- The admission order comes from upstream's `AllOrderedPlugins`
  (`pkg/kubeapiserver/options/plugins.go`); the plugins this project
  implements are matched by name, and a plugin that upstream adds and this
  project lacks fails generation.
- Field selectors (`fieldlabels.go`) from upstream's
  `AddFieldLabelConversionFunc` calls and `GetAttrs`; the subresource table
  from the discovery fixtures that `genresources` already reads; Makefile
  group lists from the generated group list.
- Version strings (`versionInfo`, the mirror's module versions) read from
  `go.mod`.

### 3. Reimplementations: shrink, and guard what stays

The 14,036 lines exist because the upstream package pulls in informers,
goroutines or dependencies that do not fit a Worker (64 MiB, about 45,000
linked functions). Three treatments, chosen per item by measuring:

- **Run upstream on a snapshot.** The controllers already do this. Apply it
  to admission plugins whose upstream package fits: construct the upstream
  plugin with listers filled from the store. Size is measured before any
  deletion.
- **Extract functions.** Where the upstream package does not fit but its
  logic is pure (LimitRanger's min/max/ratio checks, Priority resolution,
  quota evaluators, eviction's budget check), a generator copies the named
  upstream functions and the unexported helpers they reach into a
  `zz_generated` file. What stays hand-written is how objects are fetched.
- **Differential test.** For what remains hand-written, a host test feeds
  the same inputs to upstream's implementation and to this one and compares
  outcomes. Host tests have no size cap, so they can import the real plugin
  or controller. This turns silent drift at a bump into a failing test.

Order, by lines and by how well CI already covers the area: admission
built-ins (3,861), namespace deleter (882), ClusterIP allocation (1,113),
node authorizer (554), ServiceAccount tokens (593), eviction (421), garbage
collector (358), VAP type checking (365).

### 4. A bump that runs by itself

A scheduled workflow sets the next k3s patch version in `go.mod`, runs the
generators, and opens a pull request with Unit, E2E and Conformance. With
steps 1 and 2 done, a failure names a declaration or a plugin, not a hash.

## What this does not touch

The 18,860 lines of own code: the Go/Worker bridge, the loader, queue
follow-up, the Cluster Durable Object, edge routing, the supervisor. It is
not version-bound.

## Open questions for the owner

1. Step 3 changes behaviour-bearing code. Is "byte-identical outcome under
   the differential tests, then swap" the bar, or should each swap also
   wait for three Conformance runs?
2. For admission, "run upstream on a snapshot" may cost worker size. If a
   plugin does not fit, is function extraction acceptable, or should the
   admission worker be split?
3. Step 4 opens pull requests on a schedule. Wanted now, or after steps 1
   and 2?

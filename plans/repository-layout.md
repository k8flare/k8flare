# Plan: Repository layout after cloudflare-os

## Goal

Arrange the repository the way cloudflare/cloudflare-os is: Worker
configuration at the root, one flat `packages/` directory with
`{component}-{part}` names, tooling under `scripts/`, documents under
`docs/` and `plans/`.

## Locked decisions

- **Root `wrangler.jsonc`, `package.json`, `pnpm-workspace.yaml`,
  `tsconfig.json`, `.dev.vars`.** wrangler only reads `.dev.vars` next to
  its config, and the old layout made that a trap.
- **One Go module at the root.** `packages/*` are Go packages under
  `github.com/k8flare/k8flare/packages/...`; hyphenated directory names are
  fine for Go.
- **Split by concern; split large concerns as `{component}-{part}`.**
- **Move after the review, in one commit**, so the diff of the review fixes
  stays readable.

## Design

```
wrangler.jsonc            main: packages/control-plane-worker/src/index.ts
package.json / pnpm-workspace.yaml / tsconfig.json / .dev.vars(.example)
go.mod / go.sum / Makefile
scripts/                  mirror, genresources, genprinters, genopenapi, wasmpack, devtls (Go, own go.mod)
docs/  plans/
packages/
  control-plane-worker/   TS: the Worker: default fetch + APIGroups / CustomResources / OpenAPI / Scheduler / Printers entrypoints; assets/wasm (build output)
  loader-kit/             TS: Loader bootstrap and chunk assembly
  cluster-store/          TS: the Cluster Durable Object
  apiserver/              Go: the front: auth, supervisor mount, root discovery, routing; cmd/apiserver-wasm
  apiserver-<group>/      Go: one worker per served API group (generated registration + cmd); core also holds pod/node/log/namespace hooks
  apiserver-group/        Go: what the group workers share
  apiserver-installer/    Go: routes via k8s.io/apiserver's installer, per-resource implementations from registry hooks
  apiserver-registry/     Go: generic store, served table, hooks, status, field labels, tables
  apiserver-auth/         Go: token authenticators and request filters
  apiserver-kine/         Go: KineClient and the storage.Interface adapter
  apiserver-supervisor/   Go: k3s join protocol, CA vault, node identity
  customresources/        Go: upstream's CRD handler and controllers
  openapi/                Go: /openapi/v2, /openapi/v3 from the served routes
  scheduler/              Go: the real kube-scheduler, woken by the core worker
  printers/ printers-*/   Go: kubectl table printers per group
  worker-bridge/          Go: Go http.Handler <-> Loader bootstrap
  agent/                  Go: the k3s agent embedding
```

## Commit sequence (one commit)

1. `git mv` every file; update import paths, `wrangler.jsonc` paths,
   Makefile paths, the test harness's paths, README. Done 2026-09-13.
2. `make vet`, `make check`, `make test` green before committing.

## Known limitations

- Go package names still have to be identifiers: `apiserver-registry`
  declares `package registry`, and so on.

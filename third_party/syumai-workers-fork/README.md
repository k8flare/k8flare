> **k8flare fork notice**: this is a vendored fork of
> `github.com/syumai/workers` v0.32.0, wired in via a root `go.mod`
> `replace` directive so every Go WASM Worker in this repo (not just
> `workers/controllers`) picks it up. This is a permanent, ongoing
> maintenance commitment, not a throwaway spike artifact (the original
> spike copy lives at `spikes/s8-wasm-resident/vendor/syumai-workers-fork/`
> and is untouched/historical). Two changes from upstream, both required
> for `workers/controllers`' DO-hosted WASM-resident execution shape and
> harmless no-ops for every other (per-request-instantiated) Worker in
> this repo:
>
> 1. `handler_js.go`: guard the package-level `doneCh` close with
>    `sync.Once` (upstream closes it unconditionally, which panics on the
>    2nd+ request dispatched into a reused WASM instance).
> 2. `cmd/workers-assets-gen/assets/wasm_exec_go.js` (the template this
>    fork's `workers-assets-gen` copies into every project's generated
>    `wasm_exec.js`, including `workers/apiserver`'s): bind `fetch`
>    specifically (not every function) off the per-request `Proxy` so
>    plain outbound `net/http` calls (what client-go's REST transport
>    uses) don't crash with "Illegal invocation".
>
> Full verification history: `spikes/s8-wasm-resident/FINDINGS.md` and
> `docs/platform-verification.md`'s S8 section. Upstream's `_templates/`
> scaffolding was pruned from this copy (irrelevant here, and its sample
> HTML broke this repo's `vp check` formatting gate). To pick up new
> upstream `syumai/workers` releases, re-apply these two changes to a
> fresh copy rather than editing around them.

# workers

[![Go Reference](https://pkg.go.dev/badge/github.com/syumai/workers.svg)](https://pkg.go.dev/github.com/syumai/workers)
[![Discord Server](https://img.shields.io/discord/1095344956421447741?logo=discord&style=social)](https://discord.gg/tYhtatRqGs)

* `workers` is a package to run an HTTP server written in Go on [Cloudflare Workers](https://workers.cloudflare.com/).
* This package can easily serve *http.Handler* on Cloudflare Workers.
* Caution: This is an experimental project.

## Features

* [x] serve http.Handler
* [ ] R2
  - [x] Head
  - [x] Get
  - [x] Put
  - [x] Delete
  - [x] List
  - [ ] Options for R2 methods
* [ ] KV
  - [x] Get
  - [x] List
  - [x] Put
  - [x] Delete
  - [ ] Options for KV methods
* [x] Cache API
* [ ] Durable Objects
  - [x] Calling stubs
* [x] D1 (alpha)
* [x] Environment variables
* [x] FetchEvent
* [x] Cron Triggers
* [x] TCP Sockets
* [x] Queues
  - [x] Producer
  - [x] Consumer

## Installation

```
go get github.com/syumai/workers
```

## Usage

implement your http.Handler and give it to `workers.Serve()`.

```go
func main() {
	var handler http.HandlerFunc = func (w http.ResponseWriter, req *http.Request) { ... }
	workers.Serve(handler)
}
```

or just call `http.Handle` and `http.HandleFunc`, then invoke `workers.Serve()` with nil.

```go
func main() {
	http.HandleFunc("/hello", func (w http.ResponseWriter, req *http.Request) { ... })
	workers.Serve(nil) // if nil is given, http.DefaultServeMux is used.
}
```

For concrete examples, see `_examples` directory.

## Quick Start

* You can easily create and deploy a project from `Deploy to Cloudflare` button.

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https%3A%2F%2Fgithub.com%2Fsyumai%2Fworker-go-deploy)

* If you want to create a project manually, please follow the guide below.

### Requirements

* Node.js (and npm)
* Go 1.24.0 or later

### Create a new Worker project

Run the following command:

```console
npm create cloudflare@latest -- --template github.com/syumai/workers/_templates/cloudflare/worker-go
```

After creating the project, follow the steps below to initialize it.

### Initialize the project

1. Navigate to your new project directory:

```console
cd my-app
```

2. Initialize Go modules:

```console
go mod init
go mod tidy
```

3. Start the development server:

```console
npm start
```

4. Verify the worker is running:

```console
curl http://localhost:8787/hello
```

You will see **"Hello!"** as the response.

If you want a more detailed description, please refer to the README.md file in the generated directory.

## FAQ

### How do I deploy a worker implemented in this package?

To deploy a Worker, the following steps are required.

* Create a worker project using [wrangler](https://developers.cloudflare.com/workers/wrangler/).
* Build a Wasm binary.
* Upload a Wasm binary with a JavaScript code to load and instantiate Wasm (for entry point).

The [worker-go template](https://github.com/syumai/workers/tree/main/_templates/cloudflare/worker-go) contains all the required files, so I recommend using this template.

But Go (not TinyGo) with many dependencies may exceed the size limit of the Worker (3MB for free plan, 10MB for paid plan). In that case, you can use the [TinyGo template](https://github.com/syumai/workers/tree/main/_templates/cloudflare/worker-tinygo) instead.

### Where can I have discussions about contributions, or ask questions about how to use the library?

You can do both through GitHub Issues. If you want to have a more casual conversation, please use the [Discord server](https://discord.gg/tYhtatRqGs).

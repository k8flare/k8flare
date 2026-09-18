# Contributing

k8flare is pre-production and maintained by one person. Bug reports,
reproductions, and focused patches are welcome. Open an issue before
starting anything larger than a small fix so the work does not fight the
cost model.

## Rules that override everything else

1. **Idle cost must approach storage cost.** Nothing may run on an idle
   cluster: no cron, no polling Durable Object alarm, no always-on poke.
   Alarms are booked at a known deadline and deleted when there is no
   deadline.
2. **Reuse upstream Kubernetes.** New control-plane behaviour should come
   from a real `k8s.io/...` package compiled into a Worker Loader binary.

## Branches and commits

- Never commit to `main`. Use `feat/*`, `fix/*`, `docs/*`, or `chore/*`.
- Commit messages, code, and comments are in English.
- Do not add AI-tool attribution trailers.

## Checks

```
pnpm install
make test
```

`make e2e SET=required` needs a joined node and is not a PR gate yet.

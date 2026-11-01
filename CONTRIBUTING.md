# Contributing and forking

MIT means you can fork UNCUT, modify it, and ship your version. There is no CLA. You do not need permission to experiment.

## Project contract

Keep the core easy to audit and predictable in a pipeline:

- Do not silently truncate, summarize, reorder, or replace valid UTF-8 context.
- Surface provider truncation and execution failures explicitly.
- Keep stdout script-friendly: final text by default, NDJSON with `--json`.
- Prefer small, inspectable changes over new subsystems. UNCUT intentionally has no TUI, plugin framework, background agents, or automatic retries.
- Treat shell access as unsandboxed. Documentation must not imply that workspace-scoped file tools restrict shell commands.

A feature that changes context or failure behavior needs tests and clear documentation, not just a flag.

## Verify a change

```sh
gofmt -w cmd internal
go test -count=1 ./...
go vet ./...
go build -o bin/uncut ./cmd/uncut
python bench/proof.py ./bin/uncut
```

Use `bin/uncut.exe` on Windows. The proof script requires Python 3 but no provider credentials or third-party packages.

Add a regression test for every fixed behavior. Keep tests deterministic; do not require a hosted model. A separate live-model smoke test is welcome as additional evidence, not as a replacement for deterministic tests.

## Benchmarks

Measure the narrow claim you want to make. Include exact versions, OS, hardware, runtime, commands, sample counts, and raw output. Distinguish warm process launch from cold start, package size from total installation size, and CLI latency from end-to-end model latency. Do not claim superior coding quality without a suitable model evaluation.

## Pull requests

Explain the user-visible behavior, the evidence, and any trade-offs. Small PRs are easier to review. If you maintain a fork with a different contract, say so prominently so users do not confuse its behavior with UNCUT's lossless default.

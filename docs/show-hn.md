# Show HN draft

**Title:** Show HN: UNCUT – a 7 MB headless coding-agent harness with no silent context cuts

I built UNCUT to make the layer between an OpenAI-compatible model and a shell small enough to read and predictable to automate.

It is a single static Go binary with four tools: read a whole file, write a whole file, replace exactly one match, and run a shell command. The implementation is 837 lines of Go excluding tests, with no third-party Go dependencies. It runs on Linux, macOS, and Windows.

The distinguishing behavior is simple: UNCUT sends the full conversation on every model call and does not truncate file contents or combined stdout/stderr. It does not auto-compact sessions. If a provider returns `finish_reason=length`, UNCUT exits 3 instead of printing a partial answer as success. If it exhausts the configured model steps, it exits 4.

For shell and CI use, final text goes to stdout, `--json` produces NDJSON events, and optional JSONL sessions resume the complete history. It works with local Ollama models as well as hosted OpenAI-compatible endpoints.

There is deliberately no TUI, MCP, subagent system, plugin framework, telemetry, automatic retry loop, or sandbox. UNCUT does not increase the model's context window, and provider-side limits still apply.

The repository includes a dependency-free proof script that drives the real binary and checks exact provider requests. It verifies a 442,400-byte UTF-8 round trip, 213,018 bytes of command output, exact resume, edits, truncation failures, and invalid UTF-8 handling. A separate benchmark script records the methodology instead of claiming “fastest”: on my Windows machine, median `--version` process time was 9.461 ms for UNCUT versus 243.339 ms for Pi 1.4.0, with warm caches. That is a CLI launch measurement, not a model or coding-quality benchmark.

MIT. Fork it, strip it down further, or use it as a small agent loop in your own automation. I would especially value reproducible edge cases from other providers and CI setups.

Repository: https://github.com/Balanov/uncut

Proof and measurements: https://github.com/Balanov/uncut/blob/main/BENCHMARKS.md

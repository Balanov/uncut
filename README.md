<div align="center">
  <img src="assets/logo.svg" alt="UNCUT logo" width="112">
  <h1>UNCUT</h1>
  <p><strong>The lossless, headless coding agent.</strong></p>
  <p>One static binary. Four tools. Plain stdin/stdout. No silent cuts.</p>
</div>

[![CI](https://github.com/Balanov/uncut/actions/workflows/ci.yml/badge.svg)](https://github.com/Balanov/uncut/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)

UNCUT is a tiny coding-agent harness for shell pipelines, CI jobs, containers, and agent experiments. It deliberately has no TUI, telemetry, plugin system, background agents, automatic retries, or context compaction.

The entire conversation is sent on every model call. File contents and combined command output are never truncated by UNCUT. If the provider cuts a response at its output limit, UNCUT exits with an error instead of presenting a partial answer as success.

## Why UNCUT?

- **Lossless by default.** No summarization, pruning, sliding windows, or silent output caps.
- **Headless only.** Final text goes to stdout; structured NDJSON events are available with `--json`.
- **Small and portable.** One statically linked Go binary, no runtime or third-party Go dependencies.
- **Provider neutral.** Works with OpenAI-compatible Chat Completions endpoints, including local servers.
- **Resumable.** Optional append-only JSONL sessions preserve every message and tool result.
- **Forkable.** Four tools, one provider protocol, one agent loop, MIT license.

## Install

Download a binary from [Releases](https://github.com/Balanov/uncut/releases), or install with Go 1.26+:

```sh
go install github.com/Balanov/uncut/cmd/uncut@latest
```

Build from source:

```sh
git clone https://github.com/Balanov/uncut.git
cd uncut
go build -trimpath -ldflags="-s -w" -o bin/uncut ./cmd/uncut
```

On Windows, use `bin/uncut.exe` as the output path.

## Quick start

Set a model and API key:

```sh
export UNCUT_API_KEY="your-api-key"
export UNCUT_MODEL="your-model-id"
uncut "Inspect this repository and fix the failing tests"
```

For an OpenAI-compatible provider, also set its base URL:

```sh
export UNCUT_BASE_URL="https://openrouter.ai/api/v1"
```

For a local Ollama server with an installed tool-capable model:

```sh
export UNCUT_BASE_URL="http://localhost:11434/v1"
export UNCUT_MODEL="qwen3-coder"
uncut "Explain this repository"
```

No API key is required by default for local endpoints. Replace the example model with one actually installed on your machine.

### Unix pipelines

```sh
git diff | uncut "Review this diff. Do not edit files."
cat failing-test.log | uncut "Find the likely cause"
```

Piped input is appended to the prompt verbatim. UNCUT accepts valid UTF-8 text and rejects invalid input rather than replacing bytes silently.

### JSONL events

```sh
uncut --json "Run the tests and fix the failure"
```

The event stream contains `user`, `assistant`, `assistant_meta`, `tool`, and finally `done` or `error` records. Assistant messages include complete tool calls; tool messages include complete results. Usage and finish reasons are separate `assistant_meta` events.

### Sessions

```sh
uncut --session .uncut/work.jsonl "Start the refactor"
uncut --session .uncut/work.jsonl "Continue with the tests"
```

If the file exists, UNCUT resumes its complete history. New messages are appended and synced after each step. Invalid or partial records cause an explicit error; they are never discarded to make a session load.

Session files can contain source code, command output, and secrets. Store them accordingly and do not commit them accidentally.

## Configuration

| Environment variable | Purpose | Default |
| --- | --- | --- |
| `UNCUT_MODEL` | Model ID; required unless `--model` is used | none |
| `UNCUT_BASE_URL` | OpenAI-compatible base URL | `OPENAI_BASE_URL`, then `https://api.openai.com/v1` |
| `UNCUT_API_KEY` | Bearer token | `OPENAI_API_KEY`, then no token |

Useful flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `--cwd <dir>` | current directory | Workspace for file tools and shell commands |
| `--session <file>` | disabled | Create or resume an append-only JSONL session |
| `--max-steps <n>` | `32` | Maximum model calls before exit code 4 |
| `--max-tokens <n>` | provider default | Explicit provider output limit |
| `--request-timeout <duration>` | `10m` | Per-request timeout; `0` disables it |
| `--no-agents` | false | Do not read workspace `AGENTS.md` |
| `--json` | false | Emit NDJSON events instead of only final text |
| `--version` | — | Print version and exit |

`AGENTS.md` is read in full from the workspace root and added to the system prompt. On a resumed session, the original stored system message is reused.

## Tools

| Tool | Behavior |
| --- | --- |
| `read_file` | Reads an entire UTF-8 text file |
| `write_file` | Writes complete file contents and creates parent directories |
| `replace` | Replaces exactly one occurrence; zero or multiple matches fail |
| `run_command` | Returns the exit code and complete combined stdout/stderr |

File paths must stay inside the workspace. `run_command` uses `/bin/sh -c` on Unix and `cmd.exe /C` on Windows.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Successful final answer |
| `1` | Provider, I/O, session, or execution error |
| `2` | Invalid command-line usage |
| `3` | Provider returned `finish_reason=length` or `max_tokens`; partial answer rejected |
| `4` | `--max-steps` reached before a final answer |

## What “lossless” means

UNCUT does not truncate, summarize, or rewrite valid UTF-8 prompts, stored messages, file contents, or command output. It does not extend a model's context window, and it cannot control provider-side processing. When the context or output limit is exceeded, the provider may reject or cut the response; UNCUT surfaces that failure instead of hiding it.

Binary files and invalid UTF-8 are rejected explicitly. The initial release uses non-streaming Chat Completions and supports text plus tool calls, not images.

## Safety

**UNCUT is not a sandbox.** The model can execute shell commands with your operating-system user's permissions. Workspace checks on file tools do not constrain what a shell command can access. Run untrusted tasks in a disposable container or isolated environment, and review sessions and changes yourself.

There are no approval popups, automatic commits, automatic retries, or hidden recovery loops.

## Develop

```sh
go test ./...
go vet ./...
```

The test suite includes complete large-file and large-command-output round trips, session resume, provider truncation, JSON output, and platform-specific shell behavior.

```text
cmd/uncut/          CLI and process exit codes
internal/harness/   agent loop, provider, tools, sessions
assets/             logo and editable brand source
```

## License

MIT. Fork it, modify it, ship it. See [LICENSE](LICENSE).

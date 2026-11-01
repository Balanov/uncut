<div align="center">
  <img src="assets/logo.svg" alt="UNCUT logo" width="112">
  <h1>UNCUT</h1>
  <p><strong>A 7 MB lossless coding-agent harness for shell, CI, and local models.</strong></p>
  <p>No TUI. No telemetry. No silent context cuts.</p>
  <p>Linux · macOS · Windows · amd64 + arm64 · MIT</p>
</div>

[![CI](https://github.com/Balanov/uncut/actions/workflows/ci.yml/badge.svg)](https://github.com/Balanov/uncut/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Balanov/uncut)](https://github.com/Balanov/uncut/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)

UNCUT is a tiny coding-agent harness for shell pipelines, CI jobs, containers, and agent experiments. It deliberately has no TUI, telemetry, plugin system, background agents, automatic retries, or context compaction.

The entire conversation is sent on every model call. File contents and combined command output are never truncated by UNCUT. If the provider cuts a response at its output limit, UNCUT exits with an error instead of presenting a partial answer as success. [Run the proof yourself](bench/README.md) or inspect the [recorded measurements](BENCHMARKS.md).

## Why UNCUT?

- **Lossless by default.** No summarization, pruning, sliding windows, or silent output caps.
- **Headless only.** Final text goes to stdout; structured NDJSON events are available with `--json`.
- **Small and portable.** One statically linked Go binary, no runtime or third-party Go dependencies.
- **Provider neutral.** Works with OpenAI-compatible Chat Completions endpoints, including local servers.
- **Resumable.** Optional append-only JSONL sessions preserve every message and tool result.
- **Forkable.** 837 lines of implementation Go, four tools, one provider protocol, one agent loop, MIT license.

## Install

### Linux

```sh
VERSION=0.1.1
ARCH=amd64  # Use arm64 on ARM64.
curl -fSLo uncut "https://github.com/Balanov/uncut/releases/download/v${VERSION}/uncut-linux-${ARCH}"
mkdir -p "$HOME/.local/bin"
install -m 0755 uncut "$HOME/.local/bin/uncut"
export PATH="$HOME/.local/bin:$PATH"
uncut --version
```

Make sure `$HOME/.local/bin` is on your `PATH`.

### macOS

```sh
VERSION=0.1.1
ARCH=arm64  # Use amd64 on Intel.
curl -fSLo uncut "https://github.com/Balanov/uncut/releases/download/v${VERSION}/uncut-darwin-${ARCH}"
mkdir -p "$HOME/.local/bin"
install -m 0755 uncut "$HOME/.local/bin/uncut"
export PATH="$HOME/.local/bin:$PATH"
uncut --version
```

### Windows PowerShell

```powershell
$Version = "0.1.1"
$Arch = "amd64"  # Use arm64 on ARM64.
$Dest = "$env:LOCALAPPDATA\Programs\UNCUT"
New-Item -ItemType Directory -Force $Dest | Out-Null
Invoke-WebRequest -Uri "https://github.com/Balanov/uncut/releases/download/v$Version/uncut-windows-$Arch.exe" -OutFile "$Dest\uncut.exe"
$env:Path = "$Dest;$env:Path"
uncut --version
```

Add `$Dest` to your user `PATH` to make UNCUT available in future terminals.

### Checksums and OS warnings

Every release includes `checksums.txt`. For example, verify a Linux download with:

```sh
VERSION=0.1.1
ARCH=amd64
curl -fSLO "https://github.com/Balanov/uncut/releases/download/v${VERSION}/checksums.txt"
grep "uncut-linux-${ARCH}$" checksums.txt | sha256sum -c -
```

The binaries are not code-signed yet, so SmartScreen or Gatekeeper may warn. Verify the SHA-256 checksum before allowing a downloaded executable. On macOS, after verification, remove quarantine from this file only:

```sh
xattr -d com.apple.quarantine "$HOME/.local/bin/uncut"
```

You can also build from source instead.

### Go install and source builds

With Go 1.26+:

```sh
go install github.com/Balanov/uncut/cmd/uncut@latest
```

Or build a checkout:

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

For a local Ollama server with an installed tool-capable model (this exact setup was smoke-tested with `qwen3:8b`):

```sh
ollama pull qwen3:8b
export UNCUT_BASE_URL="http://localhost:11434/v1"
export UNCUT_MODEL="qwen3:8b"
uncut "Explain this repository"
```

No API key is required by default for local endpoints. In PowerShell, set variables with `$env:UNCUT_BASE_URL = "..."` and `$env:UNCUT_MODEL = "..."`. Replace the example model with one actually installed on your machine; tool-call behavior depends on the model.

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

File paths must stay inside the workspace. `run_command` uses `/bin/sh -c` on Unix and `cmd.exe /D /S /C` on Windows.

Some Windows utilities can emit UTF-16 or code-page output rather than UTF-8. UNCUT rejects that output instead of guessing a conversion; configure commands to emit UTF-8 when possible.

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

Binary files and invalid UTF-8 are rejected explicitly. UNCUT uses non-streaming Chat Completions and supports text plus tool calls, not images.

## Safety

**UNCUT is not a sandbox.** The model can execute shell commands with your operating-system user's permissions. Workspace checks on file tools do not constrain what a shell command can access. Run untrusted tasks in a disposable container or isolated environment, and review sessions and changes yourself.

There are no approval popups, automatic commits, automatic retries, or hidden recovery loops.

## Verify the lossless contract

The proof script drives the real binary against a deterministic local provider. It needs Python 3, but no API key, model, or third-party package:

```sh
go build -o bin/uncut ./cmd/uncut
python bench/proof.py ./bin/uncut
```

It checks complete large-file and command-output transport, exact session resume, edits, explicit truncation failures, step limits, and invalid UTF-8 handling. See the [recorded results](BENCHMARKS.md) and [reproduction commands](bench/README.md). These are transport tests, not model-quality benchmarks.

## Develop

```sh
gofmt -w cmd internal
go test -count=1 ./...
go vet ./...
```

The test suite includes complete large-file and large-command-output round trips, session resume, provider truncation, JSON output, and platform-specific shell behavior. See [CONTRIBUTING.md](CONTRIBUTING.md) for the project contract and PR checklist.

```text
cmd/uncut/          CLI and process exit codes
internal/harness/   agent loop, provider, tools, sessions, platform shells
bench/              lossless proof and benchmark drivers
assets/             logo and editable brand source
docs/               launch material
```

## License

MIT. Fork it, modify it, ship it. See [LICENSE](LICENSE).

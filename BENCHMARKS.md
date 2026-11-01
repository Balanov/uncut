# Reproducible measurements

These are local observations, not universal rankings. Commands and raw JSON are included so you can repeat or refute them. No benchmark on this page measures model quality, tokens, or end-to-end coding speed.

## Windows process launch and footprint

Measured on 2026-11-01 with:

- Windows 11 build 28020, AMD64; AMD Ryzen 9 5950X, 32 logical CPUs, 31.9 GiB RAM.
- UNCUT 0.1.1, built locally with Go 1.27.3 using `-trimpath -ldflags="-s -w"`.
- Pi 1.4.0 (`@earendil-works/pi-coding-agent`), installed with npm 11.12.1 and `--ignore-scripts`; Node 24.21.0 was already installed.
- Python 3.12.16 for the benchmark driver.

After five warmups per CLI, the driver measured 100 `--version` subprocess launches per CLI. Candidates were interleaved with rotating order. Every sample includes Python's subprocess overhead and uses warm OS caches. The final run had no intentional concurrent workload.

| CLI | Min | Median | p95 | Max | Measured footprint |
| --- | ---: | ---: | ---: | ---: | ---: |
| UNCUT 0.1.1 | 8.791 ms | **9.461 ms** | 10.697 ms | 11.972 ms | **7,109,120 bytes** (6.78 MiB) |
| Pi 1.4.0 | 238.001 ms | **243.339 ms** | 254.129 ms | 264.963 ms | **112,329,741 bytes** (107.13 MiB) |

For this specific `--version` workload, UNCUT's median was 25.7× lower. This does **not** mean that UNCUT completes model-driven coding tasks 25.7× faster.

The UNCUT footprint is its complete executable. Pi's footprint is the full local `node_modules` tree (121 packages), excluding the shared Node runtime. Pi's own package accounted for 23,419,059 bytes; its 355-byte `.cmd` shim alone is not a meaningful installation measurement. Pi also provides a substantially broader feature set.

Raw data: [windows-amd64-startup.json](bench/results/windows-amd64-startup.json). Reproduction commands: [bench/README.md](bench/README.md).

## Linux amd64 artifact sizes

Local UNCUT 0.1.1 was cross-built on the same Windows machine with `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`, and the flags above. The fx artifact is the official `fx-linux-x86_64.tar.gz` from release 0.0.16; its published archive SHA-256 matched before extraction.

| Artifact | Uncompressed executable | SHA-256 |
| --- | ---: | --- |
| UNCUT 0.1.1 local Linux amd64 build | **6,901,920 bytes** (6.58 MiB) | `109c18ff8dd7d6ddca5165756b9cacc16f9da2b62eb716844f3d0963f6bb3f3e` |
| fx 0.0.16 official Linux x86-64 build | **12,814,000 bytes** (12.22 MiB) | `b8670dcc121b4289e2bdcd2a3e226fd64e3def53c976c42b42de72ee69f66cae` |

fx's compressed archive was 5,680,318 bytes with SHA-256 `f0fb822c7be546c80bcc89d50521bc1454435e0c6f586e8420c4e2b84fa80599`. UNCUT is distributed as a raw binary, so this is not a like-for-like download-size comparison. fx was not executed in this Windows environment and has no official Windows release in 0.0.16.

Source artifact: [fx 0.0.16 release](https://github.com/vercel-labs/fx/releases/tag/v0.0.16).

## Lossless CLI proof

`bench/proof.py` runs the actual UNCUT executable against a deterministic loopback provider. It compares the JSON messages received by the provider, not just the final text. It needs Python 3 and no API key, model, or third-party package.

| Scenario | Verified result |
| --- | --- |
| Large UTF-8 file and piped prompt | All **442,400 bytes** arrived unchanged; the third request resumed the exact full history |
| Combined stdout/stderr | All **213,018 bytes** arrived unchanged, including explicit newlines |
| Edit and check | Exact replacement followed by a real command that exited 0 |
| Provider truncation | `finish_reason=length` produced exit 3 and zero bytes of partial stdout |
| Step limit | Two model calls with `--max-steps=2` produced explicit exit 4 |
| Invalid UTF-8 | Rejected explicitly; no replacement characters were substituted |

Raw data: [windows-amd64-proof.json](bench/results/windows-amd64-proof.json).

This suite proves UNCUT's transport and control-flow behavior for the tested cases. It does not prove that a provider preserves context internally, accepts an arbitrarily large request, or produces a correct answer.

## Local-model smoke test

A manual UNCUT 0.1.1 test with a local Ollama server and `qwen3:8b` exercised `read_file`, `replace`, `run_command`, and a resumed JSONL session in a disposable workspace. A separate filesystem assertion confirmed the edited bytes. This confirms OpenAI-compatible tool calling in that setup; it is not a model benchmark or a guarantee of tool-call reliability across models.

During an earlier smoke run, one Windows `findstr` invocation returned output that was not valid UTF-8. UNCUT rejected it explicitly; the model then used `type note.txt` and verified the edit. This is a real interoperability boundary, not a silent recovery or a lossless transcoding feature.

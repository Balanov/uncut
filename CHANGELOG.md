# Changelog

## Unreleased

## 0.2.2 — 2026-11-25


### Fixed

- `read_file`, `write_file`, and `replace` resolve symbolic links and Windows junctions before checking that a path stays inside the workspace. A link inside the workspace that pointed outside it previously let a file tool read and write outside the workspace, although the README promises that file paths stay inside it. `filepath.EvalSymlinks` is not enough for the check because it returns a junction unchanged, so link targets are read from the file system; a link that stays inside the workspace still works. Added regression tests for both directions.

## 0.2.1 - 2026-11-15


### Fixed

- `write_file` without `content` and `replace` without `new` are now refused instead of writing an empty file or deleting the matched text. A missing argument and an explicit `null` are rejected; an explicit empty string still works as before. Added regression tests.

## 0.2.0 — 2026-11-14

### Changed

- On Unix, `run_command` now starts `/bin/sh` in its own process group. Terminal signals such as Ctrl-C no longer reach the shell or its children directly; cancellation always travels through the run's context. Cancelling after the command has already finished no longer signals anything.

### Fixed

- On Unix, cancelling a run (Ctrl-C, SIGTERM) while `run_command` is executing now stops every process the shell started, not only `/bin/sh`. Previously a child such as a test runner kept running and kept the output pipe open, so UNCUT blocked until it exited on its own and ignored further signals. Added a regression test.
- The cancel path probes the process group before signalling it, so a run that has already finished is not signalled at all and a recycled process id is never hit.
- A rejected model step is no longer written to `--session`. Provider truncation, a tool call without an id or name, and an empty message previously left an unanswered `tool_call` or an empty assistant message in the stored history, so the next run on the same session sent a request that OpenAI-compatible providers reject — the session could not be resumed at all.
## 0.1.1 — 2026-11-01

### Fixed

- Preserve quoted Windows commands when invoking `cmd.exe`, including executable paths with spaces. Added a regression test using a quoted `.cmd` file and argument.

### Added

- A dependency-free, cross-platform CLI proof suite for large UTF-8 input, complete command output, exact edits, session resume, provider truncation, step limits, and invalid UTF-8.
- A reproducible process-launch and footprint benchmark driver, with recorded local results and explicit limitations.
- Per-OS installation instructions, contribution guidelines, and launch documentation.

## 0.1.0 — 2026-11-01

- Initial MIT-licensed release: one static binary, four tools, OpenAI-compatible providers, plain stdin/stdout, NDJSON events, and append-only sessions.
- Linux, macOS, and Windows builds for amd64 and arm64.

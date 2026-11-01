# Changelog

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

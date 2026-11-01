# Reproduce the checks

Both scripts use only the Python 3 standard library. They do not need API keys or make model calls. The proof provider listens only on an ephemeral loopback port.

## Lossless proof

Build UNCUT, then run the real executable:

```sh
go build -o bin/uncut ./cmd/uncut
python bench/proof.py ./bin/uncut
```

On Windows:

```powershell
go build -o bin/uncut.exe ./cmd/uncut
python bench/proof.py ./bin/uncut.exe
```

Add `--output results/proof.json` to save the report. `--work-dir PATH` chooses where temporary disposable workspaces are created.

The scripted provider checks exact request messages for:

1. A 442,400-byte UTF-8 file and piped prompt, followed by a full-history resume.
2. 213,018 bytes of combined stdout/stderr (on Windows; newlines differ on Unix).
3. An exact replacement and a real verification command.
4. Exit 3 on provider truncation, without partial stdout.
5. Exit 4 at the configured model-step limit.
6. Explicit rejection of invalid UTF-8 instead of byte replacement.

These are deterministic transport tests, not model evals.

## Process launch and footprint

Measure any installed CLIs that support `--version`:

```sh
python bench/startup.py \
  --candidate uncut=./bin/uncut \
  --candidate pi=/path/to/pi \
  --footprint pi=/path/to/pi-install-tree \
  --runs 100 --warmups 5 \
  --output results/startup.json
```

On Windows, use `uncut.exe` and the `pi.cmd` shim as the executable paths. `--footprint` is optional; without it, the script measures the executable file itself. A `.cmd` shim is not a full installation, so the Pi comparison supplies its `node_modules` tree.

To reproduce the recorded Pi 1.4.0 installation in an isolated directory:

```sh
npm install --prefix ./pi --ignore-scripts --no-audit --no-fund --save-exact \
  @earendil-works/pi-coding-agent@1.4.0
```

The script reports min, median, nearest-rank p95, and max wall time. It rotates candidate order and includes the Python subprocess overhead in every sample.

**Do not describe these numbers as cold starts.** They use warm OS caches. They do not measure provider latency, token use, model quality, or the time to complete a coding task. Record the OS, hardware, CLI versions, runtime versions, commands, and raw JSON alongside any published comparison.

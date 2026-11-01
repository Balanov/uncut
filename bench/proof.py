#!/usr/bin/env python3
"""Exercise the real UNCUT CLI against a deterministic local provider.

No API key, model download, third-party package, or network access is required.
The provider inspects the exact JSON received on every model call. A passing
result proves byte-for-byte transport through UNCUT; it does not benchmark a
model or prove anything about provider-side context handling.
"""

import argparse
import json
import os
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def call(name, arguments, call_id="proof-call"):
    return {
        "role": "assistant",
        "tool_calls": [{
            "id": call_id,
            "type": "function",
            "function": {"name": name, "arguments": json.dumps(arguments)},
        }],
    }


def text(content):
    return {"role": "assistant", "content": content}


class Provider(ThreadingHTTPServer):
    def __init__(self, script):
        super().__init__(("127.0.0.1", 0), Handler)
        self.script = script
        self.requests = []
        self.errors = []
        self.thread = threading.Thread(target=self.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *args):
        self.shutdown()
        self.server_close()
        self.thread.join()

    def verify(self, expected=None):
        expected = len(self.script) if expected is None else expected
        require(not self.errors, "provider assertions failed: " + repr(self.errors))
        require(len(self.requests) == expected,
                f"expected {expected} model calls, got {len(self.requests)}")


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        try:
            require(self.path == "/chat/completions", f"unexpected path: {self.path}")
            length = int(self.headers.get("Content-Length", "0"))
            request = json.loads(self.rfile.read(length))
            self.server.requests.append(request)
            index = len(self.server.requests) - 1
            require(index < len(self.server.script), "unexpected extra model call")
            message, finish_reason = self.server.script[index](request)
            response = json.dumps({
                "choices": [{"message": message, "finish_reason": finish_reason}],
            }).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(response)))
            self.end_headers()
            self.wfile.write(response)
        except Exception as error:
            self.server.errors.append(repr(error))
            self.send_error(500)

    def log_message(self, *args):
        pass


def invoke(binary, workspace, provider, prompt, stdin=b"", extra=()):
    env = os.environ.copy()
    for name in ("UNCUT_API_KEY", "OPENAI_API_KEY", "UNCUT_BASE_URL", "OPENAI_BASE_URL"):
        env.pop(name, None)
    command = [
        str(binary), "--model", "proof-model", "--base-url",
        f"http://127.0.0.1:{provider.server_port}", "--cwd", str(workspace),
        "--no-agents", "--request-timeout", "30s", *extra, prompt,
    ]
    return subprocess.run(command, input=stdin, capture_output=True, timeout=45, env=env)


def success(result, expected):
    require(result.returncode == 0,
            f"exit {result.returncode}: {result.stderr.decode('utf-8', 'replace')}")
    require(result.stdout.decode("utf-8") == expected + "\n", repr(result.stdout))
    require(result.stderr == b"", repr(result.stderr))


def tools(request):
    return [message for message in request["messages"] if message.get("role") == "tool"]


def prove_large_file_and_resume(binary, root):
    block = "λ雪🙂 0123456789abcdef\n"
    payload = "FILE-BEGIN\n" + block * 8192 + "FILE-MIDDLE\n" + block * 8192 + "FILE-END\n"
    (root / "large.txt").write_bytes(payload.encode("utf-8"))
    prompt = "Read every byte. Piped input follows.\n\n" + payload
    session = root / "session.jsonl"
    saved_second_request = []

    def first(request):
        require(request["messages"][1]["content"] == prompt, "piped UTF-8 input changed")
        return call("read_file", {"path": "large.txt"}), "tool_calls"

    def second(request):
        require(len(request["messages"]) == 4, "first run history is incomplete")
        require(tools(request)[0]["content"] == payload, "large file content changed")
        require(request["messages"][1]["content"] == prompt, "large prompt changed")
        saved_second_request.append(request)
        return text("FULL FILE OK"), "stop"

    def resumed(request):
        previous = saved_second_request[0]["messages"]
        require(request["messages"][:-1] == previous + [text("FULL FILE OK")],
                "resume did not preserve the exact full history")
        require(request["messages"][-1]["content"] == "Continue without losing anything.",
                "new prompt is missing")
        return text("RESUME OK"), "stop"

    with Provider([first, second, resumed]) as provider:
        result = invoke(binary, root, provider, "Read every byte. Piped input follows.",
                        stdin=payload.encode("utf-8"), extra=("--session", str(session)))
        provider.verify(2)
        success(result, "FULL FILE OK")
        result = invoke(binary, root, provider, "Continue without losing anything.",
                        extra=("--session", str(session)))
        provider.verify()
        success(result, "RESUME OK")
    return {"payload_utf8_bytes": len(payload.encode("utf-8")), "model_calls": 3}


def prove_command_output(binary, root):
    newline = "\r\n" if os.name == "nt" else "\n"
    out_line = ("stdout: 0123456789abcdef" + newline).encode()
    err_line = ("stderr: fedcba9876543210" + newline).encode()
    expected = b"COMMAND-BEGIN\n" + (out_line + err_line) * 4096 + b"COMMAND-END\n"
    code = ("import os; os.write(1,b'COMMAND-BEGIN\\n'); "
            "[(os.write(1,b'" + out_line.decode().encode("unicode_escape").decode() +
            "'),os.write(2,b'" + err_line.decode().encode("unicode_escape").decode() +
            "')) for _ in range(4096)]; os.write(1,b'COMMAND-END\\n')")
    command = f'"{sys.executable}" -c "{code}"'

    def first(request):
        return call("run_command", {"command": command}), "tool_calls"

    def second(request):
        actual = tools(request)[0]["content"].encode("utf-8")
        wanted = b"exit_code=0\n" + expected
        difference = next((i for i, pair in enumerate(zip(actual, wanted)) if pair[0] != pair[1]), None)
        require(actual == wanted,
                f"command output changed: lengths {len(actual)}/{len(wanted)}, difference {difference}, prefix {actual[:80]!r}")
        return text("FULL OUTPUT OK"), "stop"

    with Provider([first, second]) as provider:
        result = invoke(binary, root, provider, "Run the output test.")
        provider.verify()
        success(result, "FULL OUTPUT OK")
    return {"combined_output_bytes": len(expected)}


def prove_edit_and_check(binary, root):
    (root / "note.txt").write_bytes(b"status = old\n")
    check_code = ("from pathlib import Path; raise SystemExit(0 if "
                  "Path('note.txt').read_bytes() == b'status = new\\n' else 1)")
    check_command = f'"{sys.executable}" -c "{check_code}"'

    def first(request):
        return call("replace", {"path": "note.txt", "old": "status = old", "new": "status = new"}), "tool_calls"

    def second(request):
        require(tools(request)[0]["content"] == "replaced 12 bytes with 12 bytes in note.txt",
                "unexpected edit result")
        return call("run_command", {"command": check_command}, "check-call"), "tool_calls"

    def third(request):
        require(tools(request)[1]["content"] == "exit_code=0\n", "edit check failed")
        return text("EDIT OK"), "stop"

    with Provider([first, second, third]) as provider:
        result = invoke(binary, root, provider, "Edit, then check.")
        provider.verify()
        success(result, "EDIT OK")
    require((root / "note.txt").read_bytes() == b"status = new\n", "file was not edited")
    return {"verified_file_bytes": len(b"status = new\n")}


def prove_truncation_rejected(binary, root):
    with Provider([lambda request: (text("partial answer"), "length")]) as provider:
        result = invoke(binary, root, provider, "Return a partial answer.")
        provider.verify()
    require(result.returncode == 3, f"expected exit 3, got {result.returncode}")
    require(result.stdout == b"", "partial answer leaked to stdout")
    require(b"finish_reason=length" in result.stderr, "missing truncation diagnostic")
    return {"exit_code": result.returncode, "partial_stdout_bytes": len(result.stdout)}


def prove_step_limit(binary, root):
    (root / "note.txt").write_bytes(b"note\n")
    response = lambda request: (call("read_file", {"path": "note.txt"}), "tool_calls")
    with Provider([response, response]) as provider:
        result = invoke(binary, root, provider, "Keep reading.", extra=("--max-steps", "2"))
        provider.verify()
    require(result.returncode == 4, f"expected exit 4, got {result.returncode}")
    require(b"maximum model steps reached" in result.stderr, "missing step-limit diagnostic")
    return {"exit_code": result.returncode, "model_calls": 2}


def prove_invalid_output_rejected(binary, root):
    command = f'"{sys.executable}" -c "import os; os.write(1,bytes([255]))"'

    def first(request):
        return call("run_command", {"command": command}), "tool_calls"

    def second(request):
        require(tools(request)[0]["content"] ==
                "error: command output is not valid UTF-8; refusing to replace bytes silently",
                "invalid output was replaced or hidden")
        return text("INVALID OUTPUT REJECTED"), "stop"

    with Provider([first, second]) as provider:
        result = invoke(binary, root, provider, "Test invalid UTF-8.")
        provider.verify()
        success(result, "INVALID OUTPUT REJECTED")
    return {"invalid_bytes_replaced": 0}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="path to the UNCUT executable")
    parser.add_argument("--work-dir", type=Path, help="parent for temporary proof workspaces")
    parser.add_argument("--output", type=Path, help="also write the JSON report here")
    args = parser.parse_args()
    binary = args.binary.resolve()
    require(binary.is_file(), f"binary not found: {binary}")
    work_dir = args.work_dir.resolve() if args.work_dir else None
    if work_dir:
        work_dir.mkdir(parents=True, exist_ok=True)

    scenarios = [
        ("large_utf8_file_prompt_and_resume", prove_large_file_and_resume),
        ("large_combined_stdout_stderr", prove_command_output),
        ("exact_edit_and_real_check", prove_edit_and_check),
        ("provider_truncation_rejected", prove_truncation_rejected),
        ("step_limit_is_explicit", prove_step_limit),
        ("invalid_utf8_is_not_replaced", prove_invalid_output_rejected),
    ]
    report = {"suite": "uncut-lossless-proof-v1", "python": sys.version.split()[0], "scenarios": {}}
    with tempfile.TemporaryDirectory(prefix="uncut-proof-", dir=work_dir) as temporary:
        root = Path(temporary)
        for name, scenario in scenarios:
            workspace = root / name
            workspace.mkdir()
            report["scenarios"][name] = {"status": "passed", **scenario(binary, workspace)}
            print(f"PASS {name}", flush=True)
    rendered = json.dumps(report, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")


if __name__ == "__main__":
    main()

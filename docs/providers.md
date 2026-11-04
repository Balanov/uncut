# OpenAI-compatible providers

UNCUT sends non-streaming Chat Completions requests to
`<base-url>/chat/completions`. Set the base URL to the API prefix, such as
`https://provider.example/v1`, not to the full completion route.

The model ID is passed through unchanged. Choose an ID available to your
account; protocol compatibility does not guarantee that every model supports
tools or every parameter UNCUT sends. UNCUT does not discover models, retry
failed requests, or implement a streaming transport.

## Configure an endpoint

Keep credentials in the process environment, outside the repository and session
files. The CLI accepts `UNCUT_API_KEY` (falling back to `OPENAI_API_KEY`),
`UNCUT_BASE_URL` (falling back to `OPENAI_BASE_URL`), and `UNCUT_MODEL`.
`--base-url` and `--model` override their environment defaults.

For example, after setting `UNCUT_API_KEY` privately in your shell:

```sh
export UNCUT_BASE_URL='https://provider.example/v1'
export UNCUT_MODEL='your-model-id'
uncut --max-steps 1 --no-agents 'Reply with a short greeting; do not use tools.'
```

Windows PowerShell:

```powershell
$env:UNCUT_BASE_URL = 'https://provider.example/v1'
$env:UNCUT_MODEL = 'your-model-id'
uncut --max-steps 1 --no-agents 'Reply with a short greeting; do not use tools.'
```

These are configuration examples, not claims that a particular service has been
verified. Use a disposable workspace for manual checks: UNCUT shell tools run
without a sandbox. A live request can incur provider charges. Do not put real
keys in commands you share, Git commits, fixtures, logs, or bug reports.

## Failure behavior and local verification

- HTTP failures preserve their status code and response body in `ProviderError`,
  including non-JSON gateway errors. Inspect and redact diagnostics before sharing
  them; an upstream response body can contain sensitive information.
- Successful HTTP responses with invalid UTF-8, invalid JSON, or no choices are
  rejected explicitly. Invalid UTF-8 is not silently replaced with other text.
- Canceling a request during a response-body read returns a wrapped cancellation
  error and closes the body. UNCUT does not retry these requests automatically.

The provider tests use local `httptest` servers and synthetic credentials; they
need neither a hosted provider account nor internet access:

```sh
go test -count=1 ./internal/harness -run TestOpenAIProvider
```

A successful one-turn manual check establishes only that model and request's
compatibility. Tool calls, tool-result continuation, truncation, and errors need
their own checks before making broader compatibility claims.

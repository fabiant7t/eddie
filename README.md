# eddie

`eddie` is a small Go monitoring app that:

- Loads runtime configuration from environment variables and CLI flags.
- Parses one or more HTTP check specs from YAML files.
- Runs active specs in monitoring cycles (in parallel).
- Starts an HTTP server with:
  - `/` returning the status page (optionally basic-auth protected)
  - `/events` returning SSE snapshots for live status updates (basic-auth protected when configured)
  - `/healthz` returning `application/health+json` including status and app version

## Build and Run

```bash
make build
make run
```

`make build` injects build metadata (`version`, `date`, `revision`) via `-ldflags`.

## Configuration

Configuration is read with precedence: `CLI > ENV > defaults`.

### Main Settings

- `EDDIE_SPEC_PATH` / `--spec-path`  
  Path expression for spec files. Supports relative, absolute, `~`, and globs (including `**`).  
  Default: XDG path ending in `eddie/config.d`.

- `EDDIE_CYCLE_INTERVAL` / `--cycle-interval`  
  Go duration string (for example `60s`, `1m`).  
  Default: `60s`.

- `EDDIE_SHUTDOWN_TIMEOUT` / `--shutdown-timeout`  
  Go duration string (for example `5s`, `1m`).  
  Default: `5s`.

- `EDDIE_LOG_LEVEL` (or `EDDIE_LOGLEVEL`) / `--log-level`  
  One of `DEBUG`, `INFO`, `WARN`, `ERROR`.  
  Default: `INFO`.

### HTTP Server

- `EDDIE_HTTP_ADDRESS` / `--http-address` (default `0.0.0.0`)
- `EDDIE_HTTP_PORT` / `--http-port` (default `8080`)
- `EDDIE_HTTP_BASIC_AUTH_USERNAME` / `--http-basic-auth-username` (optional)
- `EDDIE_HTTP_BASIC_AUTH_PASSWORD` / `--http-basic-auth-password` (optional)

### Mail

- `EDDIE_MAIL_ENDPOINT` / `--mail-endpoint`
- `EDDIE_MAIL_PORT` / `--mail-port` (default `587`)
- `EDDIE_MAIL_USERNAME` / `--mail-username`
- `EDDIE_MAIL_PASSWORD` / `--mail-password`
- `EDDIE_MAIL_SENDER` / `--mail-sender`
- `EDDIE_MAIL_RECEIVERS` / repeated `--mail-receiver`
- `EDDIE_MAIL_NO_TLS` / `--mail-no-tls`

## Spec Format

Each YAML document is one HTTP check spec.  
One file may contain multiple spec documents separated by `---`.

### Full Example (All Fields)

```yaml
---
version: 1
http:
  name: app-health
  disabled: false
  method: GET
  follow_redirects: true
  url: https://example.com/healthz?source=eddie
  args:
    env: prod
    region: eu-central-1
  timeout: 5s
  expect:
    code: 200
    header:
      Content-Type: application/json
      Cache-Control: no-store
    body:
      exact: '{"status":"ok"}'
      contains: '"status":"ok"'
  cycles:
    failure: 3
    success: 2
  on_failure: |
    echo "[FAIL] app-health" >&2
  on_resolved: |
    echo "[RECOVERY] app-health"
```

### Minimal Example (Defaults)

```yaml
---
version: 1
http:
  name: docs-home
  url: https://example.com/
```

### Multi-Document File Example

```yaml
---
version: 1
http:
  name: api-health
  method: GET
  url: https://api.example.com/healthz
  expect:
    code: 200
---
version: 1
http:
  name: api-robots
  disabled: true
  method: GET
  url: https://api.example.com/robots.txt
```

### Field Reference

- `version`  
  Spec version field. Current examples use `1`.
- `http.name` (required)  
  Unique ID for the check (`http.name` must be unique across all parsed HTTP specs).
- `http.disabled`  
  Defaults to `false`; when `true`, the spec is parsed but not executed.
- `http.method`  
  HTTP method. Defaults to `GET` when empty.
- `http.follow_redirects`  
  Controls redirect behavior. Defaults to `false`.
- `http.url` (required)  
  Must contain scheme and host (for example `https://example.com/path`).
- `http.args`  
  Optional query parameters map. Keys overwrite same-name query params in `url`.
- `http.timeout`  
  Request timeout duration. Defaults to `5s` when omitted or set to `0`/negative.
- `http.expect.code`  
  Optional expected HTTP status code.
- `http.expect.header`  
  Optional map of response headers that must match exactly.
- `http.expect.body.exact`  
  Optional exact body match.
- `http.expect.body.contains`  
  Optional substring check in response body.
  If both `exact` and `contains` are set, both checks must pass.
- `http.cycles.failure`  
  Consecutive failure threshold before entering failing state. Defaults to `1` when omitted/`<=0`.
- `http.cycles.success`  
  Consecutive success threshold to recover from failing state. Defaults to `1` when omitted/`<=0`.
- `http.on_failure`  
  Optional shell script executed asynchronously when the spec transitions to failing.
- `http.on_resolved`  
  Optional shell script executed asynchronously when the spec transitions from failing to healthy.

## Monitoring Semantics

- Every cycle, active specs are validated concurrently (goroutines + waitgroup).
- Spec state is tracked in a state store (current implementation: in-memory).
- Failure transition:
  - occurs when `cycles.failure` consecutive checks fail.
- Recovery transition:
  - after a failure state, occurs when `cycles.success` consecutive checks succeed.
- On transition to failure:
  - `on_failure` is executed asynchronously (if configured)
  - failure email is sent to all configured mail receivers (if mail is configured)
- On transition to recovery:
  - `on_resolved` is executed asynchronously (if configured)
  - recovery email is sent to all configured mail receivers (if mail is configured)

## Parse Failure Contract

- If spec parsing fails at startup, eddie:
  - sends an email to all configured mail receivers with error details (if mail is configured)
  - exits the program

### Spec Identity Rules

- `http.name` is required and must not be empty.
- `http.name` must be unique across all parsed HTTP specs.
- Uniqueness is scoped by check type (for future types): `http.name` and `foo.name` may share the same value.

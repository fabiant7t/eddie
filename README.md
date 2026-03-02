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

### TLS Example

```yaml
---
version: 1
tls:
  name: api-cert
  host: api.example.com
  port: 443
  server_name: api.example.com
  verify: true
  reject_selfsigned: true
  min_version: "1.2"
  timeout: 5s
  cert_min_days_valid: 14
  cycles:
    failure: 2
    success: 1
```

### Probe Example

```yaml
---
version: 1
probe:
  name: hd-main-checksum-match
  requests:
    - id: left
      method: HEAD
      url: http://hgcorigin.svonm.com/hd-main.js
      args:
        cachebuster: "{unix_ts}"
    - id: right
      url: http://preview.schneevonmorgen.com/hd-main.js.md5
  extracts:
    - id: left_etag
      from: left
      source:
        type: header
        key: ETag
      transforms: ["strip_quotes"]
    - id: right_md5
      from: right
      source:
        type: body
      transforms: ["trim_space"]
  asserts:
    - id: checksum_equal
      op: eq
      left:
        ref: left_etag
      right:
        ref: right_md5
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
- `http.insecure_skip_verify`  
  When `true`, skips TLS certificate verification for HTTPS requests. Defaults to `false`.
- `http.url` (required)  
  Must contain scheme and host (for example `https://example.com/path`).
- `http.args`  
  Optional query parameters map. Keys overwrite same-name query params in `url`.
- `http.headers`  
  Optional request headers map. `Host` is supported and mapped to the request host override.
- `http.mail_receivers`  
  Optional additional email recipients for this check. Global mail receivers still receive alerts.
- `http.timeout`  
  Request timeout duration. Defaults to `5s` when omitted or set to `0`/negative.
- `http.expect.code`  
  Optional expected HTTP status code.
- `http.expect.code_any_of`  
  Optional list of acceptable HTTP status codes.
- `http.expect.header`  
  Optional map of response headers that must match exactly.
- `http.expect.header_contains`  
  Optional map of response headers that must contain the configured substring value.
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
- `tls.name` (required)  
  Unique ID for the TLS check (`tls.name` must be unique across all parsed TLS specs).
- `tls.disabled`  
  Defaults to `false`; when `true`, the spec is parsed but not executed.
- `tls.host` (required)  
  Hostname to connect to.
- `tls.port`  
  TCP port. Defaults to `443`.
- `tls.server_name`  
  Optional TLS server name for SNI/verification. Defaults to `tls.host`.
- `tls.verify`  
  Controls certificate chain verification. Defaults to `true`.
- `tls.reject_selfsigned`  
  When `true`, rejects self-signed leaf certificates even if trusted. Defaults to `true`.
- `tls.min_version`  
  Optional minimum TLS version (`"1.0"`, `"1.1"`, `"1.2"`, `"1.3"`).
- `tls.timeout`  
  Connection timeout. Defaults to `5s` when omitted or set to `0`/negative.
- `tls.cert_min_days_valid`  
  Optional minimum number of days the leaf certificate must remain valid.
- `tls.mail_receivers`  
  Optional additional email recipients for this check. Global mail receivers still receive alerts.
- `tls.cycles.failure`  
  Consecutive failure threshold before entering failing state. Defaults to `1` when omitted/`<=0`.
- `tls.cycles.success`  
  Consecutive success threshold to recover from failing state. Defaults to `1` when omitted/`<=0`.
- `tls.on_failure`  
  Optional shell script executed asynchronously when the spec transitions to failing.
- `tls.on_resolved`  
  Optional shell script executed asynchronously when the spec transitions from failing to healthy.
- `probe.name` (required)  
  Unique ID for the probe check (`probe.name` must be unique across all parsed probe specs).
- `probe.requests` (required)  
  List of HTTP requests used by the probe. Each request needs a unique `id` and a `url`.
- `probe.requests[*].method`  
  HTTP method. Defaults to `GET` when empty.
- `probe.requests[*].args`  
  Optional query parameters map. Supports `{unix_ts}` placeholder replacement.
- `probe.requests[*].headers`  
  Optional request headers map. `Host` is supported and mapped to request host override.
- `probe.requests[*].follow_redirects`  
  Controls redirect behavior. Defaults to `false`.
- `probe.requests[*].insecure_skip_verify`  
  When `true`, skips TLS certificate verification for HTTPS requests. Defaults to `false`.
- `probe.requests[*].timeout`  
  Request timeout duration. Defaults to `5s` when omitted or set to `0`/negative.
- `probe.extracts` (required)  
  Defines extracted values from request results.
- `probe.extracts[*].source.type`  
  One of `header`, `body`, `json`, `json_path`.
- `probe.extracts[*].source.key`  
  Required when `source.type` is `header` or `json_path`. For `json_path`, use dot path like `$.updated`.
- `probe.extracts[*].transforms`  
  Optional ordered transforms. Supported: `trim_space`, `strip_quotes`, `lowercase`, `as_int`, `age_seconds`.
- `probe.asserts` (required)  
  Assertion list over extracted values.
- `probe.asserts[*].op`  
  One of `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `contains`, `matches`, `all_equal`.
- `probe.asserts[*].left` / `probe.asserts[*].right`  
  Used by all ops except `all_equal`. Each operand must define exactly one of `ref` or `value`.
- `probe.asserts[*].values`  
  Used by `all_equal`, must provide at least two operands.
- `probe.mail_receivers`  
  Optional additional email recipients for this check. Global mail receivers still receive alerts.
- `probe.cycles.failure`  
  Consecutive failure threshold before entering failing state. Defaults to `1` when omitted/`<=0`.
- `probe.cycles.success`  
  Consecutive success threshold to recover from failing state. Defaults to `1` when omitted/`<=0`.
- `probe.on_failure`  
  Optional shell script executed asynchronously when the spec transitions to failing.
- `probe.on_resolved`  
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
- `probe.name` is required and must not be empty.
- `probe.name` must be unique across all parsed probe specs.
- Uniqueness is scoped by check type (for future types): `http.name` and `foo.name` may share the same value.

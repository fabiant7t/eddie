# eddie

`eddie` is a small Go monitoring app that:

- Loads runtime configuration from environment variables and CLI flags.
- Parses one or more HTTP check specs from YAML files.
- Runs active specs in monitoring cycles (in parallel).
- Starts an HTTP server with:
  - `/` returning `eddie <version>` (optionally basic-auth protected)
  - `/healthz` returning `application/health+json`
  - `/status` returning human-readable current spec states and last cycle started/completed times (basic-auth protected when configured)

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

Each YAML document represents one spec.

```yaml
---
version: 1
http:
  name: healthz
  disabled: false
  method: GET
  follow_redirects: true
  url: http://example.com/healthz
  timeout: 5s
  expect:
    code: 200
    body:
      contains: "OK"
  cycles:
    failure: 4
    success: 1
  on_failure: |
    echo "failed"
  on_success: |
    echo "ok"
```

`disabled` defaults to `false` when omitted.

`cycles.failure` and `cycles.success` both default to `1` when omitted.

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
  - `on_success` is executed asynchronously (if configured)
  - recovery email is sent to all configured mail receivers (if mail is configured)

## Parse Failure Contract

- If spec parsing fails at startup, eddie:
  - sends an email to all configured mail receivers with error details (if mail is configured)
  - exits the program

### Spec Identity Rules

- `http.name` is required and must not be empty.
- `http.name` must be unique across all parsed HTTP specs.
- Uniqueness is scoped by check type (for future types): `http.name` and `foo.name` may share the same value.

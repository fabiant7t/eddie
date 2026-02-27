# appordown

`appordown` is a small Go monitoring app that:

- Loads runtime configuration from environment variables and CLI flags.
- Parses one or more HTTP check specs from YAML files.
- Starts an HTTP server with:
  - `/` returning `app or down <version>` (optionally basic-auth protected)
  - `/healthz` returning `application/health+json`

## Build and Run

```bash
make build
make run
```

`make build` injects build metadata (`version`, `date`, `revision`) via `-ldflags`.

## Configuration

Configuration is read with precedence: `CLI > ENV > defaults`.

### Main Settings

- `APPORDOWN_SPEC_PATH` / `--spec-path`  
  Path expression for spec files. Supports relative, absolute, `~`, and globs (including `**`).  
  Default: XDG path ending in `appordown/config.d`.

- `APPORDOWN_CYCLE_INTERVAL` / `--cycle-interval`  
  Go duration string (for example `60s`, `1m`).  
  Default: `60s`.

- `APPORDOWN_LOG_LEVEL` (or `APPORDOWN_LOGLEVEL`) / `--log-level`  
  One of `DEBUG`, `INFO`, `WARN`, `ERROR`.  
  Default: `INFO`.

### HTTP Server

- `APPORDOWN_HTTP_ADDRESS` / `--http-address` (default `0.0.0.0`)
- `APPORDOWN_HTTP_PORT` / `--http-port` (default `8080`)
- `APPORDOWN_HTTP_BASIC_AUTH_USERNAME` / `--http-basic-auth-username` (optional)
- `APPORDOWN_HTTP_BASIC_AUTH_PASSWORD` / `--http-basic-auth-password` (optional)

### Mail

- `APPORDOWN_MAIL_ENDPOINT` / `--mail-endpoint`
- `APPORDOWN_MAIL_PORT` / `--mail-port` (default `587`)
- `APPORDOWN_MAIL_USERNAME` / `--mail-username`
- `APPORDOWN_MAIL_PASSWORD` / `--mail-password`
- `APPORDOWN_MAIL_SENDER` / `--mail-sender`
- `APPORDOWN_MAIL_RECEIVERS` / repeated `--mail-receiver`
- `APPORDOWN_MAIL_NO_TLS` / `--mail-no-tls`

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

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	nethttp "net/http"
	"strconv"
	"sync"
	"time"
)

var (
	statusPageTemplateOnce sync.Once
	statusPageTemplate     *template.Template
	statusPageTemplateErr  error
)

// Server holds HTTP server settings.
type Server struct {
	address           string
	port              int
	basicAuthUsername string
	basicAuthPassword string
	appVersion        string
	statusSnapshotFn  StatusSnapshotFunc
	httpServer        *nethttp.Server
}

// Option configures optional HTTP service settings.
type Option func(*Server) error

// StatusSnapshotFunc returns the latest status information for all specs.
type StatusSnapshotFunc func() StatusSnapshot

// StatusSnapshot is the data rendered by /.
type StatusSnapshot struct {
	GeneratedAt time.Time
	Specs       []SpecStatus
}

// SpecStatus is one spec row rendered by /.
type SpecStatus struct {
	Name                 string
	SourcePath           string
	Disabled             bool
	HasState             bool
	Status               string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastCycleStartedAt   time.Time
	LastCycleAt          time.Time
}

type statusRow struct {
	Name                 string `json:"name"`
	SourcePath           string `json:"source_path"`
	Disabled             bool   `json:"disabled"`
	HasState             bool   `json:"has_state"`
	State                string `json:"state"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	LastCycleStartedAt   string `json:"last_cycle_started_at"`
	LastCycleAt          string `json:"last_cycle_at"`
	StateClass           string `json:"state_class"`
}

type statusViewData struct {
	GeneratedAt string      `json:"generated_at"`
	SpecCount   int         `json:"spec_count"`
	Rows        []statusRow `json:"rows"`
}

// New creates a new HTTP server with required network settings.
func New(address string, port int, opts ...Option) (*Server, error) {
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}
	if port <= 0 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}

	server := &Server{
		address: address,
		port:    port,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(server); err != nil {
			return nil, err
		}
	}

	mux := nethttp.NewServeMux()
	mux.HandleFunc("/", server.statusHandler)
	mux.HandleFunc("/healthz", server.healthzHandler)
	mux.HandleFunc("/events", server.statusEventsHandler)

	server.httpServer = &nethttp.Server{
		Addr:    net.JoinHostPort(server.address, strconv.Itoa(server.port)),
		Handler: mux,
	}

	return server, nil
}

// WithBasicAuth configures optional HTTP basic auth credentials.
func WithBasicAuth(username, password string) Option {
	return func(s *Server) error {
		if username == "" {
			return fmt.Errorf("basic auth username is required")
		}
		if password == "" {
			return fmt.Errorf("basic auth password is required")
		}
		s.basicAuthUsername = username
		s.basicAuthPassword = password
		return nil
	}
}

// WithAppVersion configures the app version returned by healthz.
func WithAppVersion(appVersion string) Option {
	return func(s *Server) error {
		if appVersion == "" {
			return fmt.Errorf("app version cannot be empty")
		}
		s.appVersion = appVersion
		return nil
	}
}

// WithStatusSnapshot configures the status data provider used by /.
func WithStatusSnapshot(snapshotFn StatusSnapshotFunc) Option {
	return func(s *Server) error {
		if snapshotFn == nil {
			return fmt.Errorf("status snapshot function is required")
		}
		s.statusSnapshotFn = snapshotFn
		return nil
	}
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() nethttp.Handler {
	return s.httpServer.Handler
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) healthzHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/healthz" {
		nethttp.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/health+json")
	w.WriteHeader(nethttp.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "pass",
		"version": s.appVersion,
	})
}

func (s *Server) statusHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/" {
		nethttp.NotFound(w, r)
		return
	}
	if !s.requireBasicAuth(w, r) {
		return
	}
	if s.statusSnapshotFn == nil {
		nethttp.Error(w, "status endpoint is not configured", nethttp.StatusServiceUnavailable)
		return
	}

	snapshot := s.statusSnapshotFn()
	if snapshot.GeneratedAt.IsZero() {
		snapshot.GeneratedAt = time.Now().UTC()
	}
	data := buildStatusViewData(snapshot)

	const statusPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>eddie status</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #1a1b26;
      --panel: #16161e;
      --text: #c0caf5;
      --muted: #565f89;
      --border: #292e42;
      --healthy: #9ece6a;
      --failing: #f7768e;
      --unknown: #e0af68;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font: 13px/1.35 "SF Pro Text", "Segoe UI", "Helvetica Neue", Arial, sans-serif;
      background: var(--bg);
      color: var(--text);
    }
    main {
      max-width: 1300px;
      margin: 1rem auto;
      padding: 0 0.75rem;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 10px;
      overflow: hidden;
    }
    header {
      padding: 0.65rem 0.85rem;
      border-bottom: 1px solid var(--border);
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem 1rem;
      align-items: baseline;
      justify-content: space-between;
    }
    h1 {
      margin: 0;
      font-size: 0.88rem;
      font-weight: 600;
      letter-spacing: 0.01em;
      text-transform: lowercase;
    }
    .meta {
      color: var(--muted);
      font-size: 0.8rem;
    }
    .table-wrap {
      max-height: calc(100vh - 7rem);
      overflow: auto;
      padding: 0 0.6rem 0.6rem;
    }
    table {
      width: max-content;
      min-width: 100%;
      border-collapse: collapse;
      table-layout: auto;
      font-size: 0.76rem;
    }
    thead th {
      position: sticky;
      top: 0;
      z-index: 1;
      background: var(--panel);
      text-align: left;
      font-weight: 600;
      font-size: 0.68rem;
      color: var(--muted);
      letter-spacing: 0.02em;
      text-transform: uppercase;
      padding: 0.36rem 0.52rem;
      border-bottom: 1px solid var(--border);
      white-space: nowrap;
    }
    tbody td {
      padding: 0.28rem 0.52rem;
      border-bottom: 1px solid var(--border);
      vertical-align: middle;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    tbody tr:nth-child(odd) td { background: #1a1e2f; }
    tbody tr:last-child td { border-bottom: 0; }
    tbody tr:hover td { background: #1f2335; }
    code {
      font: 0.74rem/1.2 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      color: var(--muted);
    }
    .state-healthy { color: var(--healthy); font-weight: 600; }
    .state-failing { color: var(--failing); font-weight: 600; }
    .state-unknown { color: var(--unknown); font-weight: 600; }
    .bool { color: var(--muted); }
  </style>
</head>
<body>
  <main>
    <article class="panel">
      <header>
        <h1>eddie status</h1>
        <div class="meta">
          <span>generated <time id="generated-at" datetime="{{ .GeneratedAt }}">{{ .GeneratedAt }}</time></span>
          <span>•</span>
          <span id="spec-count">{{ .SpecCount }} specs</span>
          <span>•</span>
          <span id="stream-state">connecting…</span>
        </div>
      </header>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th scope="col">Name</th>
              <th scope="col">State</th>
              <th scope="col">Off</th>
              <th scope="col">State?</th>
              <th scope="col">Fail</th>
              <th scope="col">Succ</th>
              <th scope="col">Started</th>
              <th scope="col">Duration</th>
              <th scope="col">Source</th>
            </tr>
          </thead>
          <tbody id="status-rows">
            {{ range .Rows }}
            <tr>
              <td title="{{ .Name }}">{{ .Name }}</td>
              <td><span class="{{ .StateClass }}">{{ .State }}</span></td>
              <td class="bool">{{ .Disabled }}</td>
              <td class="bool">{{ .HasState }}</td>
              <td>{{ .ConsecutiveFailures }}</td>
              <td>{{ .ConsecutiveSuccesses }}</td>
              <td><time datetime="{{ .LastCycleStartedAt }}">{{ .LastCycleStartedAt }}</time></td>
              <td><time datetime="{{ .LastCycleAt }}">{{ .LastCycleAt }}</time></td>
              <td title="{{ .SourcePath }}"><code>{{ .SourcePath }}</code></td>
            </tr>
            {{ end }}
          </tbody>
        </table>
      </div>
    </article>
  </main>
  <script>
    (() => {
      const generatedAtEl = document.getElementById("generated-at");
      const specCountEl = document.getElementById("spec-count");
      const rowsEl = document.getElementById("status-rows");
      const streamStateEl = document.getElementById("stream-state");
      if (!generatedAtEl || !specCountEl || !rowsEl || !streamStateEl) return;

      function escapeHTML(value) {
        return String(value)
          .replaceAll("&", "&amp;")
          .replaceAll("<", "&lt;")
          .replaceAll(">", "&gt;")
          .replaceAll('"', "&quot;");
      }

      function stateClass(state) {
        if (state === "healthy") return "state-healthy";
        if (state === "failing") return "state-failing";
        return "state-unknown";
      }

      function setStreamState(text) {
        streamStateEl.textContent = text;
      }

      const timeFormatter = new Intl.DateTimeFormat(undefined, {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
        timeZoneName: "short",
      });

      function parseTimestamp(value) {
        if (!value || value === "never") return NaN;
        const ms = Date.parse(value);
        return Number.isNaN(ms) ? NaN : ms;
      }

      function formatStarted(value) {
        const ms = parseTimestamp(value);
        if (Number.isNaN(ms)) return value || "never";
        const formatted = timeFormatter.format(new Date(ms));
        const match = formatted.match(/(GMT[+-]\d{1,2}|UTC|[A-Z]{2,})/);
        if (!match) return formatted;
        return formatted.replace(match[1], "(" + match[1] + ")");
      }

      function formatDuration(startValue, endValue) {
        const start = parseTimestamp(startValue);
        const end = parseTimestamp(endValue);
        if (Number.isNaN(start) || Number.isNaN(end)) return "never";
        const totalMs = Math.max(0, end - start);
        if (totalMs < 1000) return totalMs + "ms";
        const totalSeconds = Math.round(totalMs / 1000);
        const hours = Math.floor(totalSeconds / 3600);
        const minutes = Math.floor((totalSeconds % 3600) / 60);
        const seconds = totalSeconds % 60;
        let out = "";
        if (hours > 0) out += hours + "h";
        if (minutes > 0 || hours > 0) out += minutes + "m";
        out += seconds + "s";
        return out;
      }

      function render(snapshot) {
        if (!snapshot || typeof snapshot !== "object") return;

        const generatedAtRaw = String(snapshot.generated_at || "unknown");
        generatedAtEl.textContent = formatStarted(generatedAtRaw);
        generatedAtEl.setAttribute("datetime", generatedAtRaw);

        const rows = Array.isArray(snapshot.rows) ? snapshot.rows : [];
        specCountEl.textContent = String(rows.length) + " specs";

        const html = rows.map((row) => {
          const name = escapeHTML(row.name ?? "");
          const sourcePath = escapeHTML(row.source_path ?? "");
          const state = escapeHTML(row.state ?? "unknown");
          const disabled = String(Boolean(row.disabled));
          const hasState = String(Boolean(row.has_state));
          const failures = escapeHTML(row.consecutive_failures ?? 0);
          const successes = escapeHTML(row.consecutive_successes ?? 0);
          const lastStartedRaw = escapeHTML(row.last_cycle_started_at ?? "never");
          const lastCompletedRaw = escapeHTML(row.last_cycle_at ?? "never");
          const lastStarted = escapeHTML(formatStarted(lastStartedRaw));
          const lastDuration = escapeHTML(formatDuration(lastStartedRaw, lastCompletedRaw));
          const cls = stateClass(row.state);

          return "<tr>"
            + "<td title=\"" + name + "\">" + name + "</td>"
            + "<td><span class=\"" + cls + "\">" + state + "</span></td>"
            + "<td class=\"bool\">" + disabled + "</td>"
            + "<td class=\"bool\">" + hasState + "</td>"
            + "<td>" + failures + "</td>"
            + "<td>" + successes + "</td>"
            + "<td><time datetime=\"" + lastStartedRaw + "\">" + lastStarted + "</time></td>"
            + "<td><time datetime=\"" + lastCompletedRaw + "\">" + lastDuration + "</time></td>"
            + "<td title=\"" + sourcePath + "\"><code>" + sourcePath + "</code></td>"
            + "</tr>";
        }).join("");

        rowsEl.innerHTML = html;
      }

      updateStaticRows();

      if (!window.EventSource) {
        setStreamState("live updates unsupported");
        return;
      }

      const stream = new EventSource("/events");
      stream.addEventListener("snapshot", (event) => {
        try {
          render(JSON.parse(event.data));
          setStreamState("live");
        } catch {
          setStreamState("parse error");
        }
      });
      stream.onopen = () => setStreamState("live");
      stream.onerror = () => setStreamState("reconnecting…");

      function updateStaticRows() {
        const generatedRaw = generatedAtEl.getAttribute("datetime") || generatedAtEl.textContent;
        generatedAtEl.textContent = formatStarted(generatedRaw);
        const rows = rowsEl.querySelectorAll("tr");
        rows.forEach((row) => {
          const timeEls = row.querySelectorAll("time");
          if (timeEls.length < 2) return;
          const startedEl = timeEls[0];
          const doneEl = timeEls[1];
          const startedRaw = startedEl.getAttribute("datetime") || "never";
          const doneRaw = doneEl.getAttribute("datetime") || "never";
          startedEl.textContent = formatStarted(startedRaw);
          doneEl.textContent = formatDuration(startedRaw, doneRaw);
        });
      }

    })();
  </script>
</body>
</html>`

	statusPageTemplateOnce.Do(func() {
		statusPageTemplate, statusPageTemplateErr = template.New("status").Parse(statusPage)
	})
	if statusPageTemplateErr != nil {
		nethttp.Error(w, "failed to render status page", nethttp.StatusInternalServerError)
		return
	}

	var rendered bytes.Buffer
	if err := statusPageTemplate.Execute(&rendered, data); err != nil {
		nethttp.Error(w, "failed to render status page", nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write(rendered.Bytes())
}

func (s *Server) statusEventsHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/events" {
		nethttp.NotFound(w, r)
		return
	}
	if !s.requireBasicAuth(w, r) {
		return
	}
	if s.statusSnapshotFn == nil {
		nethttp.Error(w, "status endpoint is not configured", nethttp.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(nethttp.Flusher)
	if !ok {
		nethttp.Error(w, "streaming is not supported", nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(nethttp.StatusOK)

	sendSnapshot := func() error {
		snapshot := s.statusSnapshotFn()
		if snapshot.GeneratedAt.IsZero() {
			snapshot.GeneratedAt = time.Now().UTC()
		}

		payload, err := json.Marshal(buildStatusViewData(snapshot))
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := sendSnapshot(); err != nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := sendSnapshot(); err != nil {
				return
			}
		}
	}
}

func buildStatusViewData(snapshot StatusSnapshot) statusViewData {
	data := statusViewData{
		GeneratedAt: snapshot.GeneratedAt.UTC().Format(time.RFC3339Nano),
		SpecCount:   len(snapshot.Specs),
		Rows:        make([]statusRow, 0, len(snapshot.Specs)),
	}

	for _, specStatus := range snapshot.Specs {
		lastCycleStarted := "never"
		if specStatus.HasState && !specStatus.LastCycleStartedAt.IsZero() {
			lastCycleStarted = specStatus.LastCycleStartedAt.UTC().Format(time.RFC3339Nano)
		}
		lastCycle := "never"
		if specStatus.HasState && !specStatus.LastCycleAt.IsZero() {
			lastCycle = specStatus.LastCycleAt.UTC().Format(time.RFC3339Nano)
		}

		status := specStatus.Status
		if status == "" {
			status = "unknown"
		}

		stateClass := "state-unknown"
		switch status {
		case "healthy":
			stateClass = "state-healthy"
		case "failing":
			stateClass = "state-failing"
		}

		data.Rows = append(data.Rows, statusRow{
			Name:                 specStatus.Name,
			SourcePath:           specStatus.SourcePath,
			Disabled:             specStatus.Disabled,
			HasState:             specStatus.HasState,
			State:                status,
			ConsecutiveFailures:  specStatus.ConsecutiveFailures,
			ConsecutiveSuccesses: specStatus.ConsecutiveSuccesses,
			LastCycleStartedAt:   lastCycleStarted,
			LastCycleAt:          lastCycle,
			StateClass:           stateClass,
		})
	}

	return data
}

func (s *Server) requireBasicAuth(w nethttp.ResponseWriter, r *nethttp.Request) bool {
	if s.basicAuthUsername == "" {
		return true
	}

	username, password, ok := r.BasicAuth()
	if ok && username == s.basicAuthUsername && password == s.basicAuthPassword {
		return true
	}

	w.Header().Set("WWW-Authenticate", `Basic realm="eddie"`)
	nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
	return false
}

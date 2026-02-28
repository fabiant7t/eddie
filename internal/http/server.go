package http

import (
	"bytes"
	"context"
	"encoding/json"
	_ "embed"
	"fmt"
	"html/template"
	"net"
	nethttp "net/http"
	"strconv"
	"sync"
	"time"
)

var (
	//go:embed status_page.html.tmpl
	statusPage string

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

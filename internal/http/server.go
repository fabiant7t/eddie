package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"
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

// StatusSnapshot is the data rendered by /status.
type StatusSnapshot struct {
	GeneratedAt time.Time
	Specs       []SpecStatus
}

// SpecStatus is one spec row rendered by /status.
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
	mux.HandleFunc("/", server.rootHandler)
	mux.HandleFunc("/healthz", server.healthzHandler)
	mux.HandleFunc("/status", server.statusHandler)

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

// WithAppVersion configures the app version returned by the root route.
func WithAppVersion(appVersion string) Option {
	return func(s *Server) error {
		if appVersion == "" {
			return fmt.Errorf("app version cannot be empty")
		}
		s.appVersion = appVersion
		return nil
	}
}

// WithStatusSnapshot configures the status data provider used by /status.
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

func (s *Server) rootHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/" {
		nethttp.NotFound(w, r)
		return
	}

	if !s.requireBasicAuth(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(fmt.Sprintf("eddie %s", s.appVersion)))
}

func (s *Server) healthzHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/healthz" {
		nethttp.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/health+json")
	w.WriteHeader(nethttp.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "pass",
	})
}

func (s *Server) statusHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/status" {
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

	var body strings.Builder
	fmt.Fprintf(&body, "generated_at=%s\n", snapshot.GeneratedAt.UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(&body, "spec_count=%d\n", len(snapshot.Specs))
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

		fmt.Fprintf(
			&body,
			"name=%s source=%s disabled=%t has_state=%t state=%s consecutive_failures=%d consecutive_successes=%d last_cycle_started_at=%s last_cycle_at=%s\n",
			specStatus.Name,
			specStatus.SourcePath,
			specStatus.Disabled,
			specStatus.HasState,
			status,
			specStatus.ConsecutiveFailures,
			specStatus.ConsecutiveSuccesses,
			lastCycleStarted,
			lastCycle,
		)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write([]byte(body.String()))
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

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	nethttp "net/http"
	"strconv"
)

// Server holds HTTP server settings.
type Server struct {
	address           string
	port              int
	basicAuthUsername string
	basicAuthPassword string
	appVersion        string
	httpServer        *nethttp.Server
}

// Option configures optional HTTP service settings.
type Option func(*Server) error

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

	if s.basicAuthUsername != "" {
		username, password, ok := r.BasicAuth()
		if !ok || username != s.basicAuthUsername || password != s.basicAuthPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="appordown"`)
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(fmt.Sprintf("app or down [%s]", s.appVersion)))
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

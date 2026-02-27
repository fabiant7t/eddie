package http

import "fmt"

// Server holds HTTP server settings.
type Server struct {
	address           string
	port              int
	basicAuthUsername string
	basicAuthPassword string
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

package mail

import "fmt"

const defaultPort = 587

// Service holds SMTP settings used for sending emails.
type Service struct {
	endpoint  string
	port      int
	username  string
	password  string
	sender    string
	receivers []string
	noTLS     bool
}

// Option configures optional mail service settings.
type Option func(*Service) error

// New creates a mail service with required SMTP parameters.
func New(endpoint, username, password, sender string, opts ...Option) (*Service, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if sender == "" {
		return nil, fmt.Errorf("sender is required")
	}

	svc := &Service{
		endpoint: endpoint,
		port:     defaultPort,
		username: username,
		password: password,
		sender:   sender,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(svc); err != nil {
			return nil, err
		}
	}

	return svc, nil
}

// WithPort overrides the default SMTP port.
func WithPort(port int) Option {
	return func(s *Service) error {
		if port <= 0 {
			return fmt.Errorf("invalid port: %d", port)
		}
		s.port = port
		return nil
	}
}

// WithReceiver appends a receiver to the receiver list.
func WithReceiver(receiver string) Option {
	return func(s *Service) error {
		if receiver == "" {
			return fmt.Errorf("receiver cannot be empty")
		}
		s.receivers = append(s.receivers, receiver)
		return nil
	}
}

// WithNoTLS disables TLS for SMTP connections.
func WithNoTLS() Option {
	return func(s *Service) error {
		s.noTLS = true
		return nil
	}
}


package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

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

// Send sends an email to a single recipient.
func (s *Service) Send(ctx context.Context, recipient string, body []byte) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if recipient == "" {
		return fmt.Errorf("recipient is required")
	}
	if len(body) == 0 {
		return fmt.Errorf("body is required")
	}

	serverAddress := net.JoinHostPort(s.endpoint, strconv.Itoa(s.port))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", serverAddress)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	client, err := smtp.NewClient(conn, s.endpoint)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if !s.noTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: s.endpoint,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("starttls failed: %w", err)
		}
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.endpoint)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}

	if err := client.Mail(s.sender); err != nil {
		return fmt.Errorf("set sender failed: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("set recipient failed: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open email data writer failed: %w", err)
	}
	defer writer.Close()

	message := formatMessage(s.sender, recipient, body)
	if _, err := writer.Write(message); err != nil {
		return fmt.Errorf("write email data failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close email data writer failed: %w", err)
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit failed: %w", err)
	}
	return nil
}

func formatMessage(sender, recipient string, body []byte) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(sender)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(recipient)
	b.WriteString("\r\n")
	b.WriteString("Subject: appordown notification\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.Write(body)
	return []byte(b.String())
}

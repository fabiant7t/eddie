package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
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
	slog.Debug("sending email",
		"endpoint", s.endpoint,
		"port", s.port,
		"recipient", recipient,
		"sender", s.sender,
	)

	if ctx == nil {
		slog.Debug("failed to send email", "error", "context is required")
		return fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		slog.Debug("failed to send email", "error", err)
		return err
	}
	if recipient == "" {
		slog.Debug("failed to send email", "error", "recipient is required")
		return fmt.Errorf("recipient is required")
	}
	if len(body) == 0 {
		slog.Debug("failed to send email", "error", "body is required")
		return fmt.Errorf("body is required")
	}

	serverAddress := net.JoinHostPort(s.endpoint, strconv.Itoa(s.port))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", serverAddress)
	if err != nil {
		slog.Debug("failed to send email", "stage", "dial", "error", err)
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

	useImplicitTLS := !s.noTLS && s.port == 465
	var client *smtp.Client
	if useImplicitTLS {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: s.endpoint,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			slog.Debug("failed to send email", "stage", "implicit_tls_handshake", "error", err)
			return fmt.Errorf("implicit tls handshake failed: %w", err)
		}
		client, err = smtp.NewClient(tlsConn, s.endpoint)
		if err != nil {
			slog.Debug("failed to send email", "stage", "smtp_client_tls", "error", err)
			return fmt.Errorf("create smtp client over tls: %w", err)
		}
	} else {
		client, err = smtp.NewClient(conn, s.endpoint)
		if err != nil {
			slog.Debug("failed to send email", "stage", "smtp_client", "error", err)
			return fmt.Errorf("create smtp client: %w", err)
		}
	}
	defer client.Close()

	if !s.noTLS && !useImplicitTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			slog.Debug("failed to send email", "stage", "starttls_extension", "error", "smtp server does not support STARTTLS")
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: s.endpoint,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			slog.Debug("failed to send email", "stage", "starttls", "error", err)
			return fmt.Errorf("starttls failed: %w", err)
		}
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.endpoint)
	if err := client.Auth(auth); err != nil {
		slog.Debug("failed to send email", "stage", "auth", "error", err)
		return fmt.Errorf("smtp auth failed: %w", err)
	}

	if err := client.Mail(s.sender); err != nil {
		slog.Debug("failed to send email", "stage", "mail_from", "error", err)
		return fmt.Errorf("set sender failed: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		slog.Debug("failed to send email", "stage", "rcpt_to", "error", err)
		return fmt.Errorf("set recipient failed: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		slog.Debug("failed to send email", "stage", "data", "error", err)
		return fmt.Errorf("open email data writer failed: %w", err)
	}
	defer writer.Close()

	message := formatMessage(s.sender, recipient, body)
	if _, err := writer.Write(message); err != nil {
		slog.Debug("failed to send email", "stage", "write", "error", err)
		return fmt.Errorf("write email data failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		slog.Debug("failed to send email", "stage", "data_close", "error", err)
		return fmt.Errorf("close email data writer failed: %w", err)
	}

	if err := client.Quit(); err != nil {
		slog.Debug("failed to send email", "stage", "quit", "error", err)
		return fmt.Errorf("smtp quit failed: %w", err)
	}
	slog.Debug("email_sent", "recipient", recipient, "endpoint", s.endpoint, "port", s.port)
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
	b.WriteString("Subject: eddie notification\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.Write(body)
	return []byte(b.String())
}

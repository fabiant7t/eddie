package mail

import (
	"context"
	"errors"
	"testing"
)

func TestNewDefaults(t *testing.T) {
	svc, err := New("smtp.example.com", "alice", "secret", "noreply@example.com")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if svc.endpoint != "smtp.example.com" {
		t.Fatalf("endpoint = %q, want %q", svc.endpoint, "smtp.example.com")
	}
	if svc.port != defaultPort {
		t.Fatalf("port = %d, want %d", svc.port, defaultPort)
	}
	if svc.username != "alice" {
		t.Fatalf("username = %q, want %q", svc.username, "alice")
	}
	if svc.password != "secret" {
		t.Fatalf("password = %q, want %q", svc.password, "secret")
	}
	if svc.sender != "noreply@example.com" {
		t.Fatalf("sender = %q, want %q", svc.sender, "noreply@example.com")
	}
	if len(svc.receivers) != 0 {
		t.Fatalf("receivers = %v, want empty", svc.receivers)
	}
	if svc.noTLS {
		t.Fatalf("noTLS = true, want false")
	}
}

func TestNewWithOptions(t *testing.T) {
	svc, err := New(
		"smtp.example.com",
		"alice",
		"secret",
		"noreply@example.com",
		WithPort(2525),
		WithReceiver("ops@example.com"),
		WithReceiver("alerts@example.com"),
		WithNoTLS(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if svc.port != 2525 {
		t.Fatalf("port = %d, want %d", svc.port, 2525)
	}
	if len(svc.receivers) != 2 {
		t.Fatalf("receivers length = %d, want %d", len(svc.receivers), 2)
	}
	if svc.receivers[0] != "ops@example.com" || svc.receivers[1] != "alerts@example.com" {
		t.Fatalf("receivers = %v, want [ops@example.com alerts@example.com]", svc.receivers)
	}
	if !svc.noTLS {
		t.Fatalf("noTLS = false, want true")
	}
}

func TestNewRequiredValidation(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		username string
		password string
		sender   string
	}{
		{name: "missing endpoint", endpoint: "", username: "u", password: "p", sender: "s"},
		{name: "missing username", endpoint: "e", username: "", password: "p", sender: "s"},
		{name: "missing password", endpoint: "e", username: "u", password: "", sender: "s"},
		{name: "missing sender", endpoint: "e", username: "u", password: "p", sender: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.endpoint, tt.username, tt.password, tt.sender)
			if err == nil {
				t.Fatalf("New() error = nil, want error")
			}
		})
	}
}

func TestNewInvalidOptions(t *testing.T) {
	_, err := New("smtp.example.com", "alice", "secret", "noreply@example.com", WithPort(0))
	if err == nil {
		t.Fatalf("New() with invalid port error = nil, want error")
	}

	_, err = New("smtp.example.com", "alice", "secret", "noreply@example.com", WithReceiver(""))
	if err == nil {
		t.Fatalf("New() with empty receiver error = nil, want error")
	}
}

func TestSendValidation(t *testing.T) {
	svc, err := New("smtp.example.com", "alice", "secret", "noreply@example.com")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = svc.Send(nil, "ops@example.com", []byte("body"))
	if err == nil {
		t.Fatalf("Send() nil context error = nil, want error")
	}

	err = svc.Send(context.Background(), "", []byte("body"))
	if err == nil {
		t.Fatalf("Send() empty recipient error = nil, want error")
	}

	err = svc.Send(context.Background(), "ops@example.com", nil)
	if err == nil {
		t.Fatalf("Send() empty body error = nil, want error")
	}
}

func TestSendCanceledContext(t *testing.T) {
	svc, err := New("smtp.example.com", "alice", "secret", "noreply@example.com")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = svc.Send(ctx, "ops@example.com", []byte("body"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() error = %v, want context.Canceled", err)
	}
}

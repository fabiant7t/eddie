package http

import "testing"

func TestNewDefaults(t *testing.T) {
	server, err := New("0.0.0.0", 8080)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if server.address != "0.0.0.0" {
		t.Fatalf("address = %q, want %q", server.address, "0.0.0.0")
	}
	if server.port != 8080 {
		t.Fatalf("port = %d, want %d", server.port, 8080)
	}
	if server.basicAuthUsername != "" {
		t.Fatalf("basicAuthUsername = %q, want empty", server.basicAuthUsername)
	}
	if server.basicAuthPassword != "" {
		t.Fatalf("basicAuthPassword = %q, want empty", server.basicAuthPassword)
	}
}

func TestNewWithBasicAuth(t *testing.T) {
	server, err := New("127.0.0.1", 9090, WithBasicAuth("admin", "secret"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if server.basicAuthUsername != "admin" {
		t.Fatalf("basicAuthUsername = %q, want %q", server.basicAuthUsername, "admin")
	}
	if server.basicAuthPassword != "secret" {
		t.Fatalf("basicAuthPassword = %q, want %q", server.basicAuthPassword, "secret")
	}
}

func TestNewValidation(t *testing.T) {
	_, err := New("", 8080)
	if err == nil {
		t.Fatalf("New() with empty address error = nil, want error")
	}

	_, err = New("0.0.0.0", 0)
	if err == nil {
		t.Fatalf("New() with invalid port error = nil, want error")
	}

	_, err = New("0.0.0.0", 8080, WithBasicAuth("", "secret"))
	if err == nil {
		t.Fatalf("New() with empty basic auth username error = nil, want error")
	}

	_, err = New("0.0.0.0", 8080, WithBasicAuth("admin", ""))
	if err == nil {
		t.Fatalf("New() with empty basic auth password error = nil, want error")
	}
}

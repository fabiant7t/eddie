package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

	_, err = New("0.0.0.0", 8080, WithAppVersion(""))
	if err == nil {
		t.Fatalf("New() with empty app version error = nil, want error")
	}
}

func TestRootRouteWithoutBasicAuth(t *testing.T) {
	server, err := New("0.0.0.0", 8080, WithAppVersion("1.2.3"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "eddie 1.2.3" {
		t.Fatalf("body = %q, want %q", got, "eddie 1.2.3")
	}
}

func TestRootRouteWithBasicAuth(t *testing.T) {
	server, err := New("0.0.0.0", 8080, WithAppVersion("1.2.3"), WithBasicAuth("admin", "secret"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	noAuthReq := httptest.NewRequest(http.MethodGet, "/", nil)
	noAuthRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(noAuthRec, noAuthReq)
	if noAuthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", noAuthRec.Code, http.StatusUnauthorized)
	}

	wrongAuthReq := httptest.NewRequest(http.MethodGet, "/", nil)
	wrongAuthReq.SetBasicAuth("admin", "wrong")
	wrongAuthRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongAuthRec, wrongAuthReq)
	if wrongAuthRec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credentials status = %d, want %d", wrongAuthRec.Code, http.StatusUnauthorized)
	}

	okReq := httptest.NewRequest(http.MethodGet, "/", nil)
	okReq.SetBasicAuth("admin", "secret")
	okRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", okRec.Code, http.StatusOK)
	}
	if got := okRec.Body.String(); got != "eddie 1.2.3" {
		t.Fatalf("body = %q, want %q", got, "eddie 1.2.3")
	}
}

func TestHealthzRouteWithoutBasicAuth(t *testing.T) {
	server, err := New("0.0.0.0", 8080)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/health+json" {
		t.Fatalf("content-type = %q, want %q", got, "application/health+json")
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if body["status"] != "pass" {
		t.Fatalf("status field = %q, want %q", body["status"], "pass")
	}
}

func TestHealthzRouteWithBasicAuthConfigured(t *testing.T) {
	server, err := New("0.0.0.0", 8080, WithBasicAuth("admin", "secret"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

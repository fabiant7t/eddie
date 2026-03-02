package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fabiant7t/eddie/internal/spec"
)

func TestValidateHTTPSpecSupportsHostHeaderAndLocationContains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "monit-test.cdn.example.com" {
			http.Error(w, "unexpected host", http.StatusBadRequest)
			return
		}
		if r.UserAgent() == "" {
			http.Error(w, "missing user-agent", http.StatusBadRequest)
			return
		}
		w.Header().Set("Location", "https://cdn.example.com/episode-002.mp3?clean=1")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	err := validateHTTPSpec(context.Background(), spec.Spec{
		HTTP: &spec.HTTPSpec{
			Name:            "monit-script-equivalent",
			Method:          http.MethodHead,
			FollowRedirects: false,
			URL:             server.URL + "/episode-002.mp3",
			Headers: map[string]string{
				"Host":       "monit-test.cdn.example.com",
				"User-Agent": "asap-monit-test",
			},
			Expect: spec.HTTPExpect{
				CodeAnyOf: []int{301, 302, 307, 308},
				HeaderContains: map[string]string{
					"Location": "clean=1",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateHTTPSpec() error = %v, want nil", err)
	}
}

func TestValidateHTTPSpecInsecureSkipVerify(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := validateHTTPSpec(context.Background(), spec.Spec{
		HTTP: &spec.HTTPSpec{
			Name: "strict-tls",
			URL:  server.URL,
			Expect: spec.HTTPExpect{
				Code: http.StatusOK,
			},
		},
	})
	if err == nil {
		t.Fatalf("validateHTTPSpec() error = nil, want certificate verification failure")
	}

	err = validateHTTPSpec(context.Background(), spec.Spec{
		HTTP: &spec.HTTPSpec{
			Name:            "insecure-tls",
			URL:             server.URL,
			InsecureSkipTLS: true,
			Expect: spec.HTTPExpect{
				Code: http.StatusOK,
			},
		},
	})
	if err != nil {
		t.Fatalf("validateHTTPSpec() with insecure TLS error = %v, want nil", err)
	}
}

func TestContainsStatusCode(t *testing.T) {
	if containsStatusCode([]int{301, 302, 307, 308}, 302) != true {
		t.Fatalf("containsStatusCode() = false, want true")
	}
	if containsStatusCode([]int{301, 302, 307, 308}, 200) != false {
		t.Fatalf("containsStatusCode() = true, want false")
	}
}

package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fabiant7t/eddie/internal/spec"
)

func TestValidateProbeSpecETagEqualsMD5WithCachebuster(t *testing.T) {
	left := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cachebuster") == "" {
			http.Error(w, "missing cachebuster", http.StatusBadRequest)
			return
		}
		w.Header().Set("ETag", "\"abc123\"")
		w.WriteHeader(http.StatusOK)
	}))
	defer left.Close()

	right := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("abc123\n"))
	}))
	defer right.Close()

	err := validateProbeSpec(context.Background(), spec.Spec{
		Probe: &spec.ProbeSpec{
			Name: "checksum-match",
			Requests: []spec.ProbeRequest{
				{
					ID:     "left",
					Method: http.MethodHead,
					URL:    left.URL + "/hd-main.js",
					Args: map[string]string{
						"cachebuster": "{unix_ts}",
					},
				},
				{
					ID:  "right",
					URL: right.URL + "/hd-main.js.md5",
				},
			},
			Extracts: []spec.ProbeExtract{
				{
					ID:   "left_etag",
					From: "left",
					Source: spec.ProbeSource{
						Type: "header",
						Key:  "ETag",
					},
					Transforms: []string{"strip_quotes"},
				},
				{
					ID:   "right_md5",
					From: "right",
					Source: spec.ProbeSource{
						Type: "body",
					},
					Transforms: []string{"trim_space"},
				},
			},
			Asserts: []spec.ProbeAssert{
				{
					ID: "equal",
					Op: "eq",
					Left: spec.ProbeOperand{
						Ref: "left_etag",
					},
					Right: spec.ProbeOperand{
						Ref: "right_md5",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateProbeSpec() error = %v, want nil", err)
	}
}

func TestValidateProbeSpecJSONAllEqual(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"b":2,"a":1}`))
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"a":1,"b":2}`))
	}))
	defer b.Close()

	err := validateProbeSpec(context.Background(), spec.Spec{
		Probe: &spec.ProbeSpec{
			Name: "json-equal",
			Requests: []spec.ProbeRequest{
				{ID: "a", URL: a.URL + "/global_config.json"},
				{ID: "b", URL: b.URL + "/global_config.json"},
			},
			Extracts: []spec.ProbeExtract{
				{
					ID:   "a_json",
					From: "a",
					Source: spec.ProbeSource{
						Type: "json",
					},
				},
				{
					ID:   "b_json",
					From: "b",
					Source: spec.ProbeSource{
						Type: "json",
					},
				},
			},
			Asserts: []spec.ProbeAssert{
				{
					ID: "same",
					Op: "all_equal",
					Values: []spec.ProbeOperand{
						{Ref: "a_json"},
						{Ref: "b_json"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateProbeSpec() error = %v, want nil", err)
	}
}

func TestValidateProbeSpecHeaderNumericAndStringAsserts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "42")
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Location", "https://edge.example.com/episode.mp3?clean=0")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	err := validateProbeSpec(context.Background(), spec.Spec{
		Probe: &spec.ProbeSpec{
			Name: "header-checks",
			Requests: []spec.ProbeRequest{
				{
					ID:              "asset",
					Method:          http.MethodHead,
					URL:             server.URL + "/hd-main.js",
					FollowRedirects: false,
				},
			},
			Extracts: []spec.ProbeExtract{
				{
					ID:   "content_length",
					From: "asset",
					Source: spec.ProbeSource{
						Type: "header",
						Key:  "Content-Length",
					},
					Transforms: []string{"as_int"},
				},
				{
					ID:   "content_type",
					From: "asset",
					Source: spec.ProbeSource{
						Type: "header",
						Key:  "Content-Type",
					},
					Transforms: []string{"trim_space"},
				},
				{
					ID:   "location",
					From: "asset",
					Source: spec.ProbeSource{
						Type: "header",
						Key:  "Location",
					},
				},
			},
			Asserts: []spec.ProbeAssert{
				{
					ID: "length-positive",
					Op: "gt",
					Left: spec.ProbeOperand{
						Ref: "content_length",
					},
					Right: spec.ProbeOperand{
						Value: 0,
					},
				},
				{
					ID: "content-type-equals",
					Op: "eq",
					Left: spec.ProbeOperand{
						Ref: "content_type",
					},
					Right: spec.ProbeOperand{
						Value: "application/javascript",
					},
				},
				{
					ID: "location-clean",
					Op: "contains",
					Left: spec.ProbeOperand{
						Ref: "location",
					},
					Right: spec.ProbeOperand{
						Value: "clean=0",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateProbeSpec() error = %v, want nil", err)
	}
}

func TestValidateProbeSpecReturnsAssertionErrorContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := validateProbeSpec(context.Background(), spec.Spec{
		Probe: &spec.ProbeSpec{
			Name: "bad-content-type",
			Requests: []spec.ProbeRequest{
				{ID: "a", Method: http.MethodHead, URL: server.URL},
			},
			Extracts: []spec.ProbeExtract{
				{
					ID:   "ct",
					From: "a",
					Source: spec.ProbeSource{
						Type: "header",
						Key:  "Content-Type",
					},
				},
			},
			Asserts: []spec.ProbeAssert{
				{
					ID: "ct-is-js",
					Op: "eq",
					Left: spec.ProbeOperand{
						Ref: "ct",
					},
					Right: spec.ProbeOperand{
						Value: "application/javascript",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("validateProbeSpec() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `assert "ct-is-js"`) {
		t.Fatalf("validateProbeSpec() error = %v, want assert id in message", err)
	}
}

func TestValidateProbeSpecJSONPathAgeSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		updated := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
		_, _ = w.Write([]byte(`{"updated":"` + updated + `"}`))
	}))
	defer server.Close()

	err := validateProbeSpec(context.Background(), spec.Spec{
		Probe: &spec.ProbeSpec{
			Name: "dms-freshness",
			Requests: []spec.ProbeRequest{
				{
					ID:  "dms",
					URL: server.URL + "/ilmenau.json",
				},
			},
			Extracts: []spec.ProbeExtract{
				{
					ID:   "updated_ts",
					From: "dms",
					Source: spec.ProbeSource{
						Type: "json_path",
						Key:  "$.updated",
					},
				},
				{
					ID:   "age_seconds",
					From: "dms",
					Source: spec.ProbeSource{
						Type: "json_path",
						Key:  "$.updated",
					},
					Transforms: []string{"age_seconds"},
				},
			},
			Asserts: []spec.ProbeAssert{
				{
					ID: "age-lte-600",
					Op: "lte",
					Left: spec.ProbeOperand{
						Ref: "age_seconds",
					},
					Right: spec.ProbeOperand{
						Value: 600,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateProbeSpec() error = %v, want nil", err)
	}
}

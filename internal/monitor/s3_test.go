package monitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fabiant7t/eddie/internal/spec"
)

func TestValidateS3SpecListCountGTE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") != "2" {
			http.Error(w, "missing list-type=2", http.StatusBadRequest)
			return
		}
		if got := r.URL.Query().Get("prefix"); got != "logs/" {
			http.Error(w, "unexpected prefix", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(listObjectsV2XML(2)))
	}))
	defer server.Close()

	two := 2
	err := validateS3Spec(context.Background(), spec.Spec{
		S3: &spec.S3Spec{
			Name:      "s3-list",
			Endpoint:  server.URL,
			PathStyle: true,
			Auth: spec.S3AuthSpec{
				Mode:            "static",
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			},
			List: &spec.S3ListSpec{
				Bucket: "bucket",
				Prefix: "logs/",
				Expect: spec.S3ListExpect{
					CountGTE: &two,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateS3Spec() error = %v, want nil", err)
	}
}

func TestValidateS3SpecListCountGTFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(listObjectsV2XML(1)))
	}))
	defer server.Close()

	two := 2
	err := validateS3Spec(context.Background(), spec.Spec{
		S3: &spec.S3Spec{
			Name:      "s3-list",
			Endpoint:  server.URL,
			PathStyle: true,
			Auth: spec.S3AuthSpec{
				Mode:            "static",
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			},
			List: &spec.S3ListSpec{
				Bucket: "bucket",
				Prefix: "logs/",
				Expect: spec.S3ListExpect{
					CountGT: &two,
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("validateS3Spec() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "want > 2") {
		t.Fatalf("validateS3Spec() error = %v, want count_gt message", err)
	}
}

func TestExpandS3Template(t *testing.T) {
	got := expandS3Template("prefix-{utc_hour_minus_1}-{utc_hour}")
	if !strings.Contains(got, "prefix-") {
		t.Fatalf("expandS3Template() = %q, want prefixed output", got)
	}
	parts := strings.Split(got, "-")
	if len(parts) < 2 {
		t.Fatalf("expandS3Template() = %q, want expanded values", got)
	}
}

func listObjectsV2XML(keyCount int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>bucket</Name>
  <Prefix>logs/</Prefix>
  <KeyCount>%d</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
</ListBucketResult>`, keyCount)
}

func TestValidateS3SpecUsesDefaultTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte(listObjectsV2XML(1)))
	}))
	defer server.Close()

	one := 1
	err := validateS3Spec(context.Background(), spec.Spec{
		S3: &spec.S3Spec{
			Name:      "s3-timeout",
			Endpoint:  server.URL,
			PathStyle: true,
			Auth: spec.S3AuthSpec{
				Mode:            "static",
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			},
			List: &spec.S3ListSpec{
				Bucket: "bucket",
				Expect: spec.S3ListExpect{
					CountEQ: &one,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateS3Spec() error = %v, want nil", err)
	}
}

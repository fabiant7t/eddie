package monitor

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fabiant7t/eddie/internal/spec"
)

func TestValidateTLSSpecRejectsMissingSpec(t *testing.T) {
	err := validateTLSSpec(context.Background(), spec.Spec{})
	if err == nil {
		t.Fatalf("validateTLSSpec() error = nil, want error")
	}
}

func TestValidateTLSSpecRejectsMissingHost(t *testing.T) {
	err := validateTLSSpec(context.Background(), spec.Spec{
		TLS: &spec.TLSSpec{Name: "tls-check"},
	})
	if err == nil {
		t.Fatalf("validateTLSSpec() error = nil, want error")
	}
}

func TestValidateTLSSpecRejectsUnknownMinVersion(t *testing.T) {
	err := validateTLSSpec(context.Background(), spec.Spec{
		TLS: &spec.TLSSpec{
			Name:       "tls-check",
			Host:       "example.com",
			MinVersion: "2.0",
		},
	})
	if err == nil {
		t.Fatalf("validateTLSSpec() error = nil, want error")
	}
}

func TestParseTLSVersion(t *testing.T) {
	testCases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "empty", raw: ""},
		{name: "trimmed", raw: " 1.2 "},
		{name: "tls10", raw: "1.0"},
		{name: "tls11", raw: "1.1"},
		{name: "tls12", raw: "1.2"},
		{name: "tls13", raw: "1.3"},
		{name: "invalid", raw: "abc", wantErr: true},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTLSVersion(tc.raw)
			if tc.wantErr && err == nil {
				t.Fatalf("parseTLSVersion(%q) error = nil, want error", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("parseTLSVersion(%q) error = %v, want nil", tc.raw, err)
			}
		})
	}
}

func TestValidateTLSSpecRejectsSelfSignedWhenConfigured(t *testing.T) {
	host, port := startSelfSignedTLSServer(t, time.Now().Add(30*24*time.Hour))
	verify := false
	rejectSelfSigned := true

	err := validateTLSSpec(context.Background(), spec.Spec{
		TLS: &spec.TLSSpec{
			Name:             "selfsigned-reject",
			Host:             host,
			Port:             port,
			Verify:           &verify,
			RejectSelfSigned: &rejectSelfSigned,
			Timeout:          2 * time.Second,
		},
	})
	if err == nil {
		t.Fatalf("validateTLSSpec() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "self-signed certificate rejected") {
		t.Fatalf("validateTLSSpec() error = %v, want self-signed rejection", err)
	}
}

func TestValidateTLSSpecRejectsExpiringCertificate(t *testing.T) {
	host, port := startSelfSignedTLSServer(t, time.Now().Add(3*24*time.Hour))
	verify := false
	rejectSelfSigned := false
	minDays := 14

	err := validateTLSSpec(context.Background(), spec.Spec{
		TLS: &spec.TLSSpec{
			Name:             "expiring-cert",
			Host:             host,
			Port:             port,
			Verify:           &verify,
			RejectSelfSigned: &rejectSelfSigned,
			CertMinDaysValid: &minDays,
			Timeout:          2 * time.Second,
		},
	})
	if err == nil {
		t.Fatalf("validateTLSSpec() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "certificate expires too soon") {
		t.Fatalf("validateTLSSpec() error = %v, want expiry rejection", err)
	}
}

func TestValidateTLSSpecAcceptsCertificateWithSufficientValidity(t *testing.T) {
	host, port := startSelfSignedTLSServer(t, time.Now().Add(30*24*time.Hour))
	verify := false
	rejectSelfSigned := false
	minDays := 14

	err := validateTLSSpec(context.Background(), spec.Spec{
		TLS: &spec.TLSSpec{
			Name:             "valid-cert",
			Host:             host,
			Port:             port,
			Verify:           &verify,
			RejectSelfSigned: &rejectSelfSigned,
			CertMinDaysValid: &minDays,
			Timeout:          2 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("validateTLSSpec() error = %v, want nil", err)
	}
}

func startSelfSignedTLSServer(t *testing.T, notAfter time.Time) (string, int) {
	t.Helper()

	certificate := selfSignedCertificate(t, notAfter)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{certificate},
	})
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				tlsConn, ok := c.(*tls.Conn)
				if !ok {
					return
				}
				_ = tlsConn.Handshake()
			}(conn)
		}
	}()

	host, portRaw, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatalf("Atoi() error = %v", err)
	}

	return host, port
}

func selfSignedCertificate(t *testing.T, notAfter time.Time) tls.Certificate {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair() error = %v", err)
	}
	return certificate
}

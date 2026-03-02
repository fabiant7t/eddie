package monitor

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/fabiant7t/eddie/internal/spec"
)

func validateTLSSpec(ctx context.Context, parsedSpec spec.Spec) error {
	tlsSpec := parsedSpec.TLS
	if tlsSpec == nil {
		return fmt.Errorf("missing tls spec")
	}

	host := strings.TrimSpace(tlsSpec.Host)
	if host == "" {
		return fmt.Errorf("tls.host is required")
	}

	port := tlsSpec.Port
	if port <= 0 {
		port = 443
	}

	timeout := tlsSpec.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	verify := true
	if tlsSpec.Verify != nil {
		verify = *tlsSpec.Verify
	}

	rejectSelfSigned := true
	if tlsSpec.RejectSelfSigned != nil {
		rejectSelfSigned = *tlsSpec.RejectSelfSigned
	}

	serverName := strings.TrimSpace(tlsSpec.ServerName)
	if serverName == "" {
		serverName = host
	}

	minVersion, err := parseTLSVersion(tlsSpec.MinVersion)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := &net.Dialer{Timeout: timeout}
	config := &tls.Config{
		InsecureSkipVerify: !verify,
		ServerName:         serverName,
		MinVersion:         minVersion,
	}
	tlsDialer := &tls.Dialer{
		NetDialer: dialer,
		Config:    config,
	}

	conn, err := tlsDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tls connect: %w", err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return fmt.Errorf("tls connection type %T does not expose tls state", conn)
	}

	_ = conn.SetDeadline(time.Now().Add(timeout))

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no peer certificates presented")
	}
	leaf := state.PeerCertificates[0]

	if rejectSelfSigned && isSelfSigned(leaf) {
		return fmt.Errorf("self-signed certificate rejected")
	}

	if tlsSpec.CertMinDaysValid != nil {
		days := *tlsSpec.CertMinDaysValid
		if days < 0 {
			return fmt.Errorf("tls.cert_min_days_valid must be >= 0")
		}
		cutoff := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		if !leaf.NotAfter.After(cutoff) {
			return fmt.Errorf("certificate expires too soon: not_after=%s", leaf.NotAfter.UTC().Format(time.RFC3339))
		}
	}

	return nil
}

func isSelfSigned(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	if !bytes.Equal(cert.RawIssuer, cert.RawSubject) {
		return false
	}
	return cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil
}

func parseTLSVersion(raw string) (uint16, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	switch value {
	case "1.0":
		return tls.VersionTLS10, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unknown tls.min_version: %q", value)
	}
}

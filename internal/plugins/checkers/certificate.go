package checkers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
)

// CertificateCheck monitors SSL/TLS certificate expiry.
type CertificateCheck struct {
	name             string
	service          string
	certPath         string // Path to certificate file (optional)
	certURL          string // URL to check certificate from (optional)
	daysBeforeExpiry int    // Alert if expiring within this many days
	timeout          time.Duration
}

// NewCertificateCheck creates a new certificate expiry health check.
// Use certPath for local file or certURL for remote TLS endpoint.
func NewCertificateCheck(name, service, certPath, certURL string, daysBeforeExpiry int, timeout time.Duration) *CertificateCheck {
	if daysBeforeExpiry <= 0 {
		daysBeforeExpiry = 30 // Default 30 days
	}
	return &CertificateCheck{
		name:             name,
		service:          service,
		certPath:         certPath,
		certURL:          certURL,
		daysBeforeExpiry: daysBeforeExpiry,
		timeout:          timeout,
	}
}

// Name returns the check's unique identifier.
func (c *CertificateCheck) Name() string { return c.name }

// Run executes the certificate check and returns the result.
func (c *CertificateCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var cert *x509.Certificate
	var err error
	var metadata map[string]string

	switch {
	case c.certURL != "":
		cert, err = c.getCertFromURL(ctx)
		metadata = map[string]string{"url": c.certURL}
	case c.certPath != "":
		cert, err = c.getCertFromFile()
		metadata = map[string]string{"path": c.certPath}
	default:
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("either certPath or certURL must be specified"),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}
	}

	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("failed to get certificate: %w", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Metadata:  metadata,
		}
	}

	timeUntilExpiry := time.Until(cert.NotAfter)
	daysUntilExpiry := int(timeUntilExpiry.Hours() / 24)

	metadata["subject"] = cert.Subject.CommonName
	metadata["issuer"] = cert.Issuer.CommonName
	metadata["expires_at"] = cert.NotAfter.Format(time.RFC3339)
	metadata["days_until_expiry"] = fmt.Sprintf("%d", daysUntilExpiry)
	metadata["threshold_days"] = fmt.Sprintf("%d", c.daysBeforeExpiry)

	if daysUntilExpiry < c.daysBeforeExpiry {
		status := checks.StatusFailed
		if daysUntilExpiry > 0 {
			status = checks.StatusDegraded // Warning state if still valid
		}
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    status,
			Error:     fmt.Errorf("certificate expires in %d days (threshold: %d days)", daysUntilExpiry, c.daysBeforeExpiry),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Metadata:  metadata,
		}
	}

	return checks.CheckResult{
		Name:      c.name,
		Service:   c.service,
		Status:    checks.StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Metadata:  metadata,
	}
}

// getCertFromURL retrieves certificate from an HTTPS endpoint.
func (c *CertificateCheck) getCertFromURL(ctx context.Context) (*x509.Certificate, error) {
	dialer := &tls.Dialer{
		Config: &tls.Config{InsecureSkipVerify: true},
	}

	conn, err := dialer.DialContext(ctx, "tcp", c.certURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", c.certURL, err)
	}
	defer conn.Close() //nolint:errcheck // best-effort close of TLS connection

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, fmt.Errorf("connection is not TLS")
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates returned from %s", c.certURL)
	}

	return certs[0], nil
}

// getCertFromFile reads certificate from a local PEM file.
func (c *CertificateCheck) getCertFromFile() (*x509.Certificate, error) {
	data, err := os.ReadFile(c.certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from %s", c.certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

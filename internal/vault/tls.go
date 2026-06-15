package vault

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CA holds a self-signed certificate authority used to sign the vault's TLS cert.
// The CA cert's fingerprint is pinned in the connection profile so clients can
// verify they're talking to the right vault without a public CA chain.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certDER []byte
}

// ServerCert holds a TLS certificate signed by a CA.
type ServerCert struct {
	cert    tls.Certificate
	certPEM []byte
	keyPEM  []byte
}

// GenerateCACert creates a new ECDSA P-256 self-signed CA certificate valid for 10 years.
func GenerateCACert() (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization:       []string{"Redoubt Vault"},
			OrganizationalUnit: []string{"Self-Signed CA"},
			CommonName:         "Redoubt Vault CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	return &CA{cert: cert, key: key, certDER: certDER}, nil
}

// Fingerprint returns the SHA-256 fingerprint of the CA cert as a hex string.
// This value is pinned in the connection profile so restic clients can verify
// the vault's TLS certificate without a public CA chain.
func (ca *CA) Fingerprint() string {
	sum := sha256.Sum256(ca.certDER)
	return hex.EncodeToString(sum[:])
}

// CertPEM returns the CA certificate in PEM format.
func (ca *CA) CertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.certDER})
}

// GenerateServerCert generates a TLS server certificate signed by this CA.
// host must be a valid IP address or hostname (used in the SAN).
func (ca *CA) GenerateServerCert(host string) (*ServerCert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate server key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Redoubt Vault"},
			CommonName:   host,
		},
		NotBefore: time.Now().Add(-1 * time.Minute),
		NotAfter:  time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	// Add SAN for the host.
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("create server certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal server key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("build TLS key pair: %w", err)
	}

	return &ServerCert{cert: tlsCert, certPEM: certPEM, keyPEM: keyPEM}, nil
}

// WriteTo writes the CA certificate and key to dir/ca.crt and dir/ca.key.
func (ca *CA) WriteTo(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(ca.key)
	if err != nil {
		return fmt.Errorf("marshal CA key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), ca.CertPEM(), 0644); err != nil {
		return fmt.Errorf("write ca.crt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.key"), keyPEM, 0600); err != nil {
		return fmt.Errorf("write ca.key: %w", err)
	}
	return nil
}

// WriteTo writes the server certificate and key to dir/server.crt and dir/server.key.
func (sc *ServerCert) WriteTo(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "server.crt"), sc.certPEM, 0644); err != nil {
		return fmt.Errorf("write server.crt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.key"), sc.keyPEM, 0600); err != nil {
		return fmt.Errorf("write server.key: %w", err)
	}
	return nil
}

func randomSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	return serial, nil
}

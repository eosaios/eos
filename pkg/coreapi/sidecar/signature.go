package sidecar

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	SignatureAlgorithmEd25519 = "ed25519"
	SignaturePrefix           = "ed25519:"
	SignaturePlaceholder      = "unsigned-development-placeholder"
)

var (
	ErrSignatureInvalid     = errors.New("core sidecar signature invalid")
	ErrSignaturePlaceholder = errors.New("core sidecar signature is development placeholder, not a verified release signature")
)

func BuildSignedPayload(m Manifest) string {
	features := append([]string(nil), m.Features...)
	sort.Strings(features)
	return strings.Join([]string{
		m.CoreVersion,
		m.APIVersion,
		m.Target,
		m.Binary,
		m.SHA256,
		strings.Join(features, ","),
		m.MinCLIVersion,
	}, "|")
}

func IsPlaceholderSignature(sig string) bool {
	return strings.TrimSpace(sig) == SignaturePlaceholder
}

func (m Manifest) CheckSignature(publicKey ed25519.PublicKey, allowDevPlaceholder bool) error {
	sig := strings.TrimSpace(m.Signature)
	if sig == "" {
		return ErrSignatureMissing
	}
	if IsPlaceholderSignature(sig) {
		if allowDevPlaceholder {
			return nil
		}
		return ErrSignaturePlaceholder
	}
	if publicKey != nil {
		return VerifySignature(m, publicKey)
	}
	return nil
}

func VerifySignature(m Manifest, publicKey ed25519.PublicKey) error {
	sig := strings.TrimSpace(m.Signature)
	if sig == "" {
		return ErrSignatureMissing
	}
	if IsPlaceholderSignature(sig) {
		return ErrSignaturePlaceholder
	}
	if m.SignatureAlgorithm != "" && m.SignatureAlgorithm != SignatureAlgorithmEd25519 {
		return fmt.Errorf("%w: unsupported signature_algorithm %q (expected %q)", ErrSignatureInvalid, m.SignatureAlgorithm, SignatureAlgorithmEd25519)
	}
	if !strings.HasPrefix(sig, SignaturePrefix) {
		return fmt.Errorf("%w: unsupported signature format (expected %s<base64>)", ErrSignatureInvalid, SignaturePrefix)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sig[len(SignaturePrefix):])
	if err != nil {
		return fmt.Errorf("%w: invalid base64 in signature: %v", ErrSignatureInvalid, err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature length %d bytes, want %d bytes", ErrSignatureInvalid, len(sigBytes), ed25519.SignatureSize)
	}
	payload := BuildSignedPayload(m)
	if !ed25519.Verify(publicKey, []byte(payload), sigBytes) {
		return fmt.Errorf("%w: Ed25519 signature verification failed", ErrSignatureInvalid)
	}
	return nil
}

func SignManifest(m *Manifest, privateKey ed25519.PrivateKey) {
	payload := BuildSignedPayload(*m)
	sig := ed25519.Sign(privateKey, []byte(payload))
	m.Signature = SignaturePrefix + base64.StdEncoding.EncodeToString(sig)
	m.SignatureAlgorithm = SignatureAlgorithmEd25519
}

func LoadPublicKeyFromPEM(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in public key data")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key PEM: %w", err)
	}
	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("PEM key is %T, want ed25519.PublicKey", key)
	}
	return pub, nil
}

func LoadPublicKeyFromFile(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file %s: %w", path, err)
	}
	return LoadPublicKeyFromPEM(data)
}

func LoadPrivateKeyFromPEM(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key data")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key PEM: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("PEM key is %T, want ed25519.PrivateKey", key)
	}
	return priv, nil
}

// defaultPublicKeyPEM is the embedded Ed25519 public key for signature verification.
//
// Production signing key (rotated 2026-09-06 from the smoke-test key, which had
// no custodied private half). The matching private key lives only in GitHub
// Secrets (EOS_SIGNING_KEY: eos-cli / eos-core-rs / eos-app-src release
// workflows) and the release custodian's offline backup — never in any repo.
// Override paths (in order of precedence):
//  1. EOS_SIGNATURE_PUBLIC_KEY env var pointing at a PEM file
//  2. ResolveOptions.PublicKeyPath passed to ResolveBinary
//  3. defaultPublicKeyPEM (this constant)
const defaultPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEA7C5Imzul3OHXb6YSNwq8xPGwddCP/0Oy5gA+CfsTxf0=
-----END PUBLIC KEY-----`

func DefaultPublicKey() ed25519.PublicKey {
	block, _ := pem.Decode([]byte(defaultPublicKeyPEM))
	if block == nil {
		return nil
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil
	}
	return edPub
}

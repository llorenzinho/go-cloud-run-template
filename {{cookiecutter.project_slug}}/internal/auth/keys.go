package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ParsePublicKeyPEM parses the first PEM block of data: a PKIX "PUBLIC KEY",
// a PKCS#1 "RSA PUBLIC KEY" or a "CERTIFICATE" (its public key). Only RSA,
// ECDSA and Ed25519 keys are accepted; private keys are rejected.
func ParsePublicKeyPEM(data []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("public key: no PEM block found")
	}
	var (
		key any
		err error
	)
	switch block.Type {
	case "PUBLIC KEY":
		key, err = x509.ParsePKIXPublicKey(block.Bytes)
	case "RSA PUBLIC KEY":
		key, err = x509.ParsePKCS1PublicKey(block.Bytes)
	case "CERTIFICATE":
		var cert *x509.Certificate
		if cert, err = x509.ParseCertificate(block.Bytes); err == nil {
			key = cert.PublicKey
		}
	default:
		if strings.Contains(block.Type, "PRIVATE") {
			return nil, errors.New("public key: got a private key: configure the public key only")
		}
		return nil, fmt.Errorf("public key: unsupported PEM block %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	switch key.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey:
		return key, nil
	default:
		return nil, fmt.Errorf("public key: unsupported key type %T", key)
	}
}

// roles reads the roles at the dotted path of claims: an array of strings, an
// object whose keys are the roles, or a single string. A missing claim means
// no roles; any other shape is an error.
func roles(claims map[string]any, path string) ([]string, error) {
	if path == "" {
		return []string{}, nil
	}
	var cur any = claims
	for part := range strings.SplitSeq(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return []string{}, nil
		}
		if cur, ok = m[part]; !ok {
			return []string{}, nil
		}
	}
	switch v := cur.(type) {
	case nil:
		return []string{}, nil
	case string:
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, r := range v {
			s, ok := r.(string)
			if !ok {
				return nil, fmt.Errorf("roles claim %q: expected strings, got %T", path, r)
			}
			out = append(out, s)
		}
		return out, nil
	case map[string]any:
		return slices.Sorted(maps.Keys(v)), nil
	default:
		return nil, fmt.Errorf("roles claim %q: unexpected type %T", path, cur)
	}
}

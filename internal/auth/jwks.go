package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
)

// Only verification-relevant metadata is decoded; private key material is rejected.
type publicJWK struct {
	KeyID     string          `json:"kid"`
	KeyType   string          `json:"kty"`
	Algorithm string          `json:"alg,omitempty"`
	Use       string          `json:"use,omitempty"`
	Modulus   string          `json:"n"`
	Exponent  string          `json:"e"`
	D         json.RawMessage `json:"d,omitempty"`
	P         json.RawMessage `json:"p,omitempty"`
	Q         json.RawMessage `json:"q,omitempty"`
	DP        json.RawMessage `json:"dp,omitempty"`
	DQ        json.RawMessage `json:"dq,omitempty"`
	QI        json.RawMessage `json:"qi,omitempty"`
	Other     json.RawMessage `json:"oth,omitempty"`
	KeyOps    []string        `json:"key_ops,omitempty"`
}

type publicKeySet struct {
	Keys []publicJWK `json:"keys"`
}

func decodePublicKeys(set publicKeySet) (map[string]*rsa.PublicKey, error) {
	if len(set.Keys) == 0 || len(set.Keys) > 16 {
		return nil, errors.New("auth JWKS: invalid key count")
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, jwk := range set.Keys {
		if len(jwk.D)+len(jwk.P)+len(jwk.Q)+len(jwk.DP)+len(jwk.DQ)+len(jwk.QI)+len(jwk.Other) != 0 {
			return nil, errors.New("auth JWKS: private key material is not permitted")
		}
		if jwk.KeyType != "RSA" || (jwk.Use != "" && jwk.Use != "sig") || (jwk.Algorithm != "" && jwk.Algorithm != "RS256") {
			continue
		}
		if !validValue(jwk.KeyID) || len(jwk.KeyID) > 64 || keys[jwk.KeyID] != nil || (len(jwk.KeyOps) > 0 && (len(jwk.KeyOps) != 1 || jwk.KeyOps[0] != "verify")) {
			return nil, errors.New("auth JWKS: invalid signing key metadata")
		}
		n, err := base64.RawURLEncoding.Strict().DecodeString(jwk.Modulus)
		if err != nil || len(n) < 256 || len(n) > 512 {
			return nil, errors.New("auth JWKS: invalid RSA modulus")
		}
		e, err := base64.RawURLEncoding.Strict().DecodeString(jwk.Exponent)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return nil, errors.New("auth JWKS: invalid RSA exponent")
		}
		modulus, exponent := new(big.Int).SetBytes(n), new(big.Int).SetBytes(e).Int64()
		if modulus.BitLen() < 2048 || modulus.Bit(0) == 0 || exponent < 3 || exponent > 2147483647 || exponent%2 == 0 {
			return nil, errors.New("auth JWKS: weak or invalid RSA public key")
		}
		keys[jwk.KeyID] = &rsa.PublicKey{N: modulus, E: int(exponent)}
	}
	if len(keys) == 0 {
		return nil, errors.New("auth JWKS: no supported RSA signing keys")
	}
	return keys, nil
}

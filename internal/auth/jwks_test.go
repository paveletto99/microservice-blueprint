package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
)

// Key publication belongs only to test fixtures; production only consumes external JWKS.
func encodePublicJWK(kid string, key *rsa.PublicKey) publicJWK {
	return publicJWK{KeyID: kid, KeyType: "RSA", Algorithm: "RS256", Use: "sig", Modulus: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), Exponent: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}
}

func TestExternalJWKSFilteringAndPrivateMaterial(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	valid := encodePublicJWK("supported", &key.PublicKey)
	withoutOptionalMetadata := valid
	withoutOptionalMetadata.Use, withoutOptionalMetadata.Algorithm = "", ""
	keys, err := decodePublicKeys(publicKeySet{Keys: []publicJWK{{KeyID: "other-algorithm", KeyType: "EC", Algorithm: "ES256"}, withoutOptionalMetadata}})
	if err != nil || len(keys) != 1 || keys["supported"] == nil {
		t.Fatal("supported key was lost among other provider key types")
	}
	for _, mutate := range []func(*publicJWK){
		func(k *publicJWK) { k.D = json.RawMessage(`"fixture"`) },
		func(k *publicJWK) { k.P = json.RawMessage(`"fixture"`) },
		func(k *publicJWK) { k.Other = json.RawMessage(`[]`) },
		func(k *publicJWK) { k.KeyOps = []string{"sign"} },
	} {
		invalid := valid
		mutate(&invalid)
		if _, err := decodePublicKeys(publicKeySet{Keys: []publicJWK{invalid}}); err == nil {
			t.Fatal("private material or invalid signing usage was accepted")
		}
	}
}

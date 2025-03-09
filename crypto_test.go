package hpke

import (
	"bytes"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"log"
	"testing"

	"github.com/cloudflare/circl/dh/sidh"
)

func randomBytes(size int) []byte {
	out := make([]byte, size)
	rand.Read(out)
	return out
}

// P256_SHA3_256_AES_GCM_256(密钥交换算法：P256;哈希算法：SHA3-256;对称加密算法：AES-GCM-256)
func TestKEMSchemes(t *testing.T) {
	schemes := []KEMScheme{
		&dhkemScheme{group: x25519Scheme{}, KDF: hkdfScheme{hash: crypto.SHA256}},
		&dhkemScheme{group: x448Scheme{}, KDF: hkdfScheme{hash: crypto.SHA512}},
		&dhkemScheme{group: ecdhScheme{curve: elliptic.P256()}, KDF: hkdfScheme{hash: crypto.SHA256}},
		&dhkemScheme{group: ecdhScheme{curve: elliptic.P521()}, KDF: hkdfScheme{hash: crypto.SHA512}},
		&dhkemScheme{group: ecdhScheme{curve: elliptic.P256()}, KDF: hkdfScheme{hash: crypto.SHA3_256}},
		&sikeScheme{field: sidh.Fp503, KDF: hkdfScheme{hash: crypto.SHA512}},
		&sikeScheme{field: sidh.Fp751, KDF: hkdfScheme{hash: crypto.SHA512}},
	}

	for i, s := range schemes {
		log.Printf("Testing scheme %d", i)

		// Generate key pair
		log.Printf("Scheme %d: Generating KEM key pair", i)
		skR, pkR, err := s.GenerateKeyPair(rand.Reader)
		if err != nil {
			t.Fatalf("Scheme %d: Error generating KEM key pair: %v", i, err)
		}

		log.Printf("Scheme %d: KEM key pair generated successfully", i)

		// 输出公钥和私钥
		log.Printf("Scheme %d: Public Key: %s", i, hex.EncodeToString(s.Marshal(pkR)))
		log.Printf("Scheme %d: Private Key: %s", i, hex.EncodeToString(s.MarshalPrivate(skR)))

		// Encapsulation
		log.Printf("Scheme %d: Performing KEM encapsulation", i)
		zzI, enc, err := s.Encap(rand.Reader, pkR)
		if err != nil {
			t.Fatalf("Scheme %d: Error in KEM encapsulation: %v", i, err)
		}
		log.Printf("Scheme %d: KEM encapsulation completed successfully", i)

		// Decapsulation
		log.Printf("Scheme %d: Performing KEM decapsulation", i)
		zzR, err := s.Decap(enc, skR)
		if err != nil {
			t.Fatalf("Scheme %d: Error in KEM decapsulation: %v", i, err)
		}
		log.Printf("Scheme %d: KEM decapsulation completed successfully", i)
		// Verify results
		log.Printf("Scheme %d: Verifying KEM results", i)
		if !bytes.Equal(zzI, zzR) {
			t.Fatalf("Scheme %d: Asymmetric KEM results [%x] != [%x]", i, zzI, zzR)
		}
		log.Printf("Scheme %d: KEM results verification successful", i)

	}

}

func TestDHSchemes(t *testing.T) {
	schemes := []dhScheme{
		ecdhScheme{curve: elliptic.P256()},
		ecdhScheme{curve: elliptic.P521()},
		x25519Scheme{},
		x448Scheme{},
	}

	for i, s := range schemes {
		skA, pkA, err := s.GenerateKeyPair(rand.Reader)
		if err != nil {
			t.Fatalf("[%d] Error generating DH key pair: %v", i, err)
		}

		skB, pkB, err := s.GenerateKeyPair(rand.Reader)
		if err != nil {
			t.Fatalf("[%d] Error generating DH key pair: %v", i, err)
		}

		enc := s.Marshal(pkA)
		_, err = s.Unmarshal(enc)
		if err != nil {
			t.Fatalf("[%d] Error parsing DH public key: %v", i, err)
		}

		zzAB, err := s.DH(skA, pkB)
		if err != nil {
			t.Fatalf("[%d] Error performing DH operation: %v", i, err)
		}

		zzBA, err := s.DH(skB, pkA)
		if err != nil {
			t.Fatalf("[%d] Error performing DH operation: %v", i, err)
		}

		if !bytes.Equal(zzAB, zzBA) {
			t.Fatalf("[%d] Asymmetric DH results [%x] != [%x]", i, zzAB, zzBA)
		}

		if len(s.Marshal(pkA)) != len(s.Marshal(pkB)) {
			t.Fatalf("[%d] Non-constant public key size [%x] != [%x]", i, len(s.Marshal(pkA)), len(s.Marshal(pkB)))
		}
	}
}

func TestAEADSchemes(t *testing.T) {
	schemes := []AEADScheme{
		aesgcmScheme{keySize: 16},
		aesgcmScheme{keySize: 32},
		chachaPolyScheme{},
	}

	for i, s := range schemes {
		key := randomBytes(int(s.KeySize()))
		nonce := randomBytes(int(s.NonceSize()))
		pt := randomBytes(1024)
		aad := randomBytes(1024)

		aead, err := s.New(key)
		if err != nil {
			t.Fatalf("[%d] Error instantiating AEAD: %v", i, err)
		}

		ctWithAAD := aead.Seal(nil, nonce, pt, aad)
		ptWithAAD, err := aead.Open(nil, nonce, ctWithAAD, aad)
		if err != nil {
			t.Fatalf("[%d] Error decrypting with AAD: %v", i, err)
		}

		if !bytes.Equal(ptWithAAD, pt) {
			t.Fatalf("[%d] Incorrect decryption [%x] != [%x]", i, ptWithAAD, pt)
		}

		ctWithoutAAD := aead.Seal(nil, nonce, pt, nil)
		ptWithoutAAD, err := aead.Open(nil, nonce, ctWithoutAAD, nil)
		if err != nil {
			t.Fatalf("[%d] Error decrypting without AAD: %v", i, err)
		}

		if !bytes.Equal(ptWithoutAAD, pt) {
			t.Fatalf("[%d] Incorrect decryption [%x] != [%x]", i, ptWithoutAAD, pt)
		}

		if bytes.Equal(ctWithAAD, ctWithoutAAD) {
			t.Fatalf("[%d] AAD not included in ciphertext", i)
		}
	}
}

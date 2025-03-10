package hpke

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/hmac"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	_ "crypto/sha256"
	_ "crypto/sha3"
	_ "crypto/sha512"

	"git.schwanenlied.me/yawning/x448.git"
	"github.com/cloudflare/circl/dh/sidh"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

////////
// DHKEM

type dhScheme interface {
	ID() KEMID
	GenerateKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error)
	Marshal(pk KEMPublicKey) []byte
	Unmarshal(enc []byte) (KEMPublicKey, error)
	DH(priv KEMPrivateKey, pub KEMPublicKey) ([]byte, error)
	PublicKeySize() int

	MarshalPrivate(sk KEMPrivateKey) []byte
	UnmarshalPrivate(enc []byte) (KEMPrivateKey, error)
}

type dhkemScheme struct {
	group dhScheme
	KDF   KDFScheme
	skE   KEMPrivateKey
}

func (s dhkemScheme) ID() KEMID {
	return s.group.ID()
}

func (s dhkemScheme) GenerateKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error) {
	return s.group.GenerateKeyPair(rand)
}

func (s dhkemScheme) Marshal(pk KEMPublicKey) []byte {
	return s.group.Marshal(pk)
}

func (s dhkemScheme) MarshalPrivate(sk KEMPrivateKey) []byte {
	return s.group.MarshalPrivate(sk)
}

func (s dhkemScheme) Unmarshal(enc []byte) (KEMPublicKey, error) {
	return s.group.Unmarshal(enc)
}

func (s dhkemScheme) UnmarshalPrivate(enc []byte) (KEMPrivateKey, error) {
	return s.group.UnmarshalPrivate(enc)
}

func (s *dhkemScheme) setEphemeralKeyPair(skE KEMPrivateKey) {
	s.skE = skE
}

func (s dhkemScheme) getEphemeralKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error) {
	if s.skE != nil {
		return s.skE, s.skE.PublicKey(), nil
	}

	return s.group.GenerateKeyPair(rand)
}

func (s dhkemScheme) extractAndExpand(dh []byte, kemContext []byte, Nzz int) []byte {
	prk := s.KDF.LabeledExtract(nil, "dh", dh)
	return s.KDF.LabeledExpand(prk, "prk", kemContext, Nzz)
}

func (s dhkemScheme) Encap(rand io.Reader, pkR KEMPublicKey) ([]byte, []byte, error) {
	skE, pkE, err := s.getEphemeralKeyPair(rand)
	if err != nil {
		return nil, nil, err
	}

	dh, err := s.group.DH(skE, pkR)
	if err != nil {
		return nil, nil, err
	}

	enc := s.group.Marshal(pkE)
	pkRm := s.group.Marshal(pkR)

	kemContext := make([]byte, len(enc)+len(pkRm))
	copy(kemContext, enc)
	copy(kemContext[len(enc):], pkRm)

	Nzz := s.KDF.OutputSize()
	zz := s.extractAndExpand(dh, kemContext, Nzz)

	return zz, enc, nil
}

func (s dhkemScheme) Decap(enc []byte, skR KEMPrivateKey) ([]byte, error) {
	pkE, err := s.group.Unmarshal(enc)
	if err != nil {
		return nil, err
	}

	dh, err := s.group.DH(skR, pkE)
	if err != nil {
		return nil, err
	}

	pkRm := s.group.Marshal(skR.PublicKey())

	kemContext := make([]byte, len(enc)+len(pkRm))
	copy(kemContext, enc)
	copy(kemContext[len(enc):], pkRm)

	Nzz := s.KDF.OutputSize()
	zz := s.extractAndExpand(dh, kemContext, Nzz)

	return zz, nil
}

func (s dhkemScheme) AuthEncap(rand io.Reader, pkR KEMPublicKey, skS KEMPrivateKey) ([]byte, []byte, error) {
	skE, pkE, err := s.getEphemeralKeyPair(rand)
	if err != nil {
		return nil, nil, err
	}

	dhER, err := s.group.DH(skE, pkR)
	if err != nil {
		return nil, nil, err
	}

	dhIR, err := s.group.DH(skS, pkR)
	if err != nil {
		return nil, nil, err
	}

	dh := append(dhER, dhIR...)

	enc := s.group.Marshal(pkE)
	pkRm := s.group.Marshal(pkR)
	pkSm := s.group.Marshal(skS.PublicKey())

	Nenc := len(enc)
	Npk := len(pkRm)
	Nsk := len(pkSm)
	kemContext := make([]byte, Nenc+Npk+Nsk)
	copy(kemContext[:Nenc], enc)
	copy(kemContext[Nenc:Nenc+Npk], pkRm)
	copy(kemContext[Nenc+Npk:], pkSm)

	Nzz := s.KDF.OutputSize()
	zz := s.extractAndExpand(dh, kemContext, Nzz)

	return zz, enc, nil
}

func (s dhkemScheme) AuthDecap(enc []byte, skR KEMPrivateKey, pkS KEMPublicKey) ([]byte, error) {
	pkE, err := s.group.Unmarshal(enc)
	if err != nil {
		return nil, err
	}

	dhER, err := s.group.DH(skR, pkE)
	if err != nil {
		return nil, err
	}

	dhIR, err := s.group.DH(skR, pkS)
	if err != nil {
		return nil, err
	}

	dh := append(dhER, dhIR...)

	pkRm := s.group.Marshal(skR.PublicKey())
	pkSm := s.group.Marshal(pkS)

	Nenc := len(enc)
	Npk := len(pkRm)
	Nsk := len(pkSm)
	kemContext := make([]byte, Nenc+Npk+Nsk)
	copy(kemContext[:Nenc], enc)
	copy(kemContext[Nenc:Nenc+Npk], pkRm)
	copy(kemContext[Nenc+Npk:], pkSm)

	Nzz := s.KDF.OutputSize()
	zz := s.extractAndExpand(dh, kemContext, Nzz)

	return zz, nil
}

func (s dhkemScheme) PublicKeySize() int {
	return s.group.PublicKeySize()
}

////////////////////////
// ECDH with NIST curves

type ecdhPrivateKey struct {
	curve elliptic.Curve
	d     []byte
	x, y  *big.Int
}

func (priv ecdhPrivateKey) PublicKey() KEMPublicKey {
	return &ecdhPublicKey{priv.curve, priv.x, priv.y}
}

type ecdhPublicKey struct {
	curve elliptic.Curve
	x, y  *big.Int
}

type ecdhScheme struct {
	curve elliptic.Curve
	skE   KEMPrivateKey
}

func (s ecdhScheme) ID() KEMID {
	switch s.curve.Params().Name {
	case "P-256":
		return DHKEM_P256
	case "P-521":
		return DHKEM_P521
	}
	panic(fmt.Sprintf("Unsupported curve: %s", s.curve.Params().Name))
}

func (s ecdhScheme) GenerateKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error) {
	d, x, y, err := elliptic.GenerateKey(s.curve, rand)
	if err != nil {
		return nil, nil, err
	}

	priv := &ecdhPrivateKey{s.curve, d, x, y}
	return priv, priv.PublicKey(), nil
}

func (s ecdhScheme) Marshal(pk KEMPublicKey) []byte {
	if pk == nil {
		return nil
	}
	raw := pk.(*ecdhPublicKey)
	return elliptic.Marshal(raw.curve, raw.x, raw.y)
}

func (s ecdhScheme) MarshalPrivate(sk KEMPrivateKey) []byte {
	if sk == nil {
		return nil
	}

	raw := sk.(*ecdhPrivateKey)
	copied := make([]byte, len(raw.d))
	copy(copied, raw.d)
	return copied
}

func (s ecdhScheme) Unmarshal(enc []byte) (KEMPublicKey, error) {
	x, y := elliptic.Unmarshal(s.curve, enc)
	if x == nil {
		return nil, fmt.Errorf("Error unmarshaling public key")
	}

	return &ecdhPublicKey{s.curve, x, y}, nil
}

func (s ecdhScheme) UnmarshalPrivate(enc []byte) (KEMPrivateKey, error) {
	if enc == nil {
		return nil, fmt.Errorf("Invalid input")
	}

	x, y := s.curve.Params().ScalarBaseMult(enc)
	return &ecdhPrivateKey{s.curve, enc, x, y}, nil
}

func (s ecdhScheme) DH(priv KEMPrivateKey, pub KEMPublicKey) ([]byte, error) {
	ecdhPriv, ok := priv.(*ecdhPrivateKey)
	if !ok {
		return nil, fmt.Errorf("Private key not suitable for ECDH")
	}

	ecdhPub, ok := pub.(*ecdhPublicKey)
	if !ok {
		return nil, fmt.Errorf("Public key not suitable for ECDH")
	}

	x, y := s.curve.Params().ScalarMult(ecdhPub.x, ecdhPub.y, ecdhPriv.d)
	dh := elliptic.Marshal(ecdhPub.curve, x, y)

	return dh, nil
}

func (s ecdhScheme) PublicKeySize() int {
	feSize := (s.curve.Params().BitSize + 7) >> 3
	return 1 + 2*feSize
}

///////////////////
// ECDH with X25519

type x25519PrivateKey struct {
	val [32]byte
}

func (priv x25519PrivateKey) PublicKey() KEMPublicKey {
	pub := &x25519PublicKey{}
	curve25519.ScalarBaseMult(&pub.val, &priv.val)
	return pub
}

type x25519PublicKey struct {
	val [32]byte
}

type x25519Scheme struct {
	skE KEMPrivateKey
}

func (s x25519Scheme) ID() KEMID {
	return DHKEM_X25519
}

func (s x25519Scheme) GenerateKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error) {
	priv := &x25519PrivateKey{}
	_, err := rand.Read(priv.val[:])
	if err != nil {
		return nil, nil, err
	}

	return priv, priv.PublicKey(), nil
}

func (s x25519Scheme) Marshal(pk KEMPublicKey) []byte {
	if pk == nil {
		return nil
	}
	raw := pk.(*x25519PublicKey)
	return raw.val[:]
}

func (s x25519Scheme) MarshalPrivate(sk KEMPrivateKey) []byte {
	if sk == nil {
		return nil
	}
	raw := sk.(*x25519PrivateKey)
	return raw.val[:]
}

func (s x25519Scheme) Unmarshal(enc []byte) (KEMPublicKey, error) {
	if len(enc) != 32 {
		return nil, fmt.Errorf("Error unmarshaling X25519 public key")
	}

	pub := &x25519PublicKey{}
	copy(pub.val[:], enc)
	return pub, nil
}

func (s x25519Scheme) UnmarshalPrivate(enc []byte) (KEMPrivateKey, error) {
	if enc == nil {
		return nil, fmt.Errorf("Invalid input")
	}

	if len(enc) != 32 {
		return nil, fmt.Errorf("Error unmarshaling X25519 private key")
	}

	key := &x25519PrivateKey{}
	copy(key.val[:], enc[0:32])
	return key, nil
}

func (s x25519Scheme) DH(priv KEMPrivateKey, pub KEMPublicKey) ([]byte, error) {
	xPriv, ok := priv.(*x25519PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Private key not suitable for X25519: %+v", priv)
	}

	xPub, ok := pub.(*x25519PublicKey)
	if !ok {
		return nil, fmt.Errorf("Private key not suitable for X25519")
	}

	// TODO ScalarMult
	var zz [32]byte
	curve25519.ScalarMult(&zz, &xPriv.val, &xPub.val)
	return zz[:], nil
}

func (s x25519Scheme) PublicKeySize() int {
	return 32
}

///////////////////
// ECDH with X448

type x448PrivateKey struct {
	val [56]byte
}

func (priv x448PrivateKey) PublicKey() KEMPublicKey {
	pub := &x448PublicKey{}
	x448.ScalarBaseMult(&pub.val, &priv.val)
	return pub
}

type x448PublicKey struct {
	val [56]byte
}

type x448Scheme struct {
	skE KEMPrivateKey
}

func (s x448Scheme) ID() KEMID {
	return DHKEM_X448
}

func (s x448Scheme) GenerateKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error) {
	priv := &x448PrivateKey{}
	_, err := rand.Read(priv.val[:])
	if err != nil {
		return nil, nil, err
	}

	return priv, priv.PublicKey(), nil
}

func (s x448Scheme) Marshal(pk KEMPublicKey) []byte {
	if pk == nil {
		return nil
	}
	raw := pk.(*x448PublicKey)
	return raw.val[:]
}

func (s x448Scheme) MarshalPrivate(sk KEMPrivateKey) []byte {
	if sk == nil {
		return nil
	}
	raw := sk.(*x448PrivateKey)
	return raw.val[:]
}

func (s x448Scheme) Unmarshal(enc []byte) (KEMPublicKey, error) {
	if len(enc) != 56 {
		return nil, fmt.Errorf("Error unmarshaling X448 public key")
	}

	pub := &x448PublicKey{}
	copy(pub.val[:], enc)
	return pub, nil
}

func (s x448Scheme) UnmarshalPrivate(enc []byte) (KEMPrivateKey, error) {
	if enc == nil {
		return nil, fmt.Errorf("Invalid input")
	}

	if len(enc) != 56 {
		return nil, fmt.Errorf("Error unmarshaling X448 private key")
	}

	key := &x448PrivateKey{}
	copy(key.val[:], enc[0:56])
	return key, nil
}

func (s x448Scheme) DH(priv KEMPrivateKey, pub KEMPublicKey) ([]byte, error) {
	xPriv, ok := priv.(*x448PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Private key not suitable for X448: %+v", priv)
	}

	xPub, ok := pub.(*x448PublicKey)
	if !ok {
		return nil, fmt.Errorf("Public key not suitable for X448: %+v", pub)
	}

	var zz [56]byte
	x448.ScalarMult(&zz, &xPriv.val, &xPub.val)
	return zz[:], nil
}

func (s x448Scheme) PublicKeySize() int {
	return 56
}

///////
// SIKE

type sikePublicKey struct {
	field uint8
	pub   *sidh.PublicKey
}

type sikePrivateKey struct {
	field uint8
	priv  *sidh.PrivateKey
	pub   *sidh.PublicKey
}

func (priv sikePrivateKey) PublicKey() KEMPublicKey {
	return &sikePublicKey{priv.field, priv.pub}
}

type sikeScheme struct {
	field uint8
	KDF   KDFScheme
}

func (s sikeScheme) ID() KEMID {
	switch s.field {
	case sidh.Fp503:
		return KEM_SIKE503
	case sidh.Fp751:
		return KEM_SIKE751
	}
	panic(fmt.Sprintf("Unsupported field: %d", s.field))
}

func (s sikeScheme) GenerateKeyPair(rand io.Reader) (KEMPrivateKey, KEMPublicKey, error) {
	rawPriv := sidh.NewPrivateKey(s.field, sidh.KeyVariantSike)
	err := rawPriv.Generate(rand)
	if err != nil {
		return nil, nil, err
	}

	rawPub := sidh.NewPublicKey(s.field, sidh.KeyVariantSike)
	rawPriv.GeneratePublicKey(rawPub)

	priv := &sikePrivateKey{s.field, rawPriv, rawPub}
	return priv, priv.PublicKey(), nil
}

func (s sikeScheme) Marshal(pk KEMPublicKey) []byte {
	if pk == nil {
		return nil
	}
	raw := pk.(*sikePublicKey)
	out := make([]byte, raw.pub.Size())
	raw.pub.Export(out)
	return out
}

func (s sikeScheme) MarshalPrivate(sk KEMPrivateKey) []byte {
	panic("Not implemented")
	return nil
}

func (s sikeScheme) Unmarshal(enc []byte) (KEMPublicKey, error) {
	rawPub := sidh.NewPublicKey(s.field, sidh.KeyVariantSike)
	if len(enc) != rawPub.Size() {
		return nil, fmt.Errorf("Invalid public key size: got %d, expected %d", len(enc), rawPub.Size())
	}

	err := rawPub.Import(enc)
	if err != nil {
		return nil, err
	}

	return &sikePublicKey{s.field, rawPub}, nil
}

func (s sikeScheme) UnmarshalPrivate(enc []byte) (KEMPrivateKey, error) {
	panic("Not implemented")
	return nil, nil
}

func (s sikeScheme) newKEM(rand io.Reader) (*sidh.KEM, error) {
	switch s.field {
	case sidh.Fp503:
		return sidh.NewSike503(rand), nil
	case sidh.Fp751:
		return sidh.NewSike751(rand), nil
	}
	return nil, fmt.Errorf("Invalid field")
}

func (s sikeScheme) Encap(rand io.Reader, pkR KEMPublicKey) ([]byte, []byte, error) {
	raw := pkR.(*sikePublicKey)

	kem, err := s.newKEM(rand)
	if err != nil {
		return nil, nil, err
	}

	enc := make([]byte, kem.CiphertextSize())
	zz := make([]byte, s.KDF.OutputSize())
	err = kem.Encapsulate(enc, zz, raw.pub)
	if err != nil {
		return nil, nil, err
	}

	return zz, enc, nil
}

type panicReader struct{}

func (p panicReader) Read(unused []byte) (int, error) {
	panic("Should not read")
}

func (s sikeScheme) Decap(enc []byte, skR KEMPrivateKey) ([]byte, error) {
	raw := skR.(*sikePrivateKey)

	kem, err := s.newKEM(panicReader{})
	if err != nil {
		return nil, err
	}

	zz := make([]byte, s.KDF.OutputSize())
	err = kem.Decapsulate(zz, raw.priv, raw.pub, enc)
	if err != nil {
		return nil, err
	}
	//shared secret key
	return zz, nil
}

func (s sikeScheme) PublicKeySize() int {
	rawPub := sidh.NewPublicKey(s.field, sidh.KeyVariantSike)
	return rawPub.Size()
}

func (s sikeScheme) setEphemeralKeyPair(skE KEMPrivateKey) {
	panic("SIKE cannot use a pre-set ephemeral key pair")
}

//////////
// AES-GCM

type aesgcmScheme struct {
	keySize int
}

func (s aesgcmScheme) ID() AEADID {
	switch s.keySize {
	case 16:
		return AEAD_AESGCM128
	case 32:
		return AEAD_AESGCM256
	}
	panic(fmt.Sprintf("Unsupported key size: %d", s.keySize))

}

func (s aesgcmScheme) New(key []byte) (cipher.AEAD, error) {
	if len(key) != s.keySize {
		return nil, fmt.Errorf("Incorrect key size %d != %d", len(key), s.keySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

func (s aesgcmScheme) KeySize() int {
	return s.keySize
}

func (s aesgcmScheme) NonceSize() int {
	return 12
}

//////////
// ChaCha20-Poly1305

type chachaPolyScheme struct {
}

func (s chachaPolyScheme) ID() AEADID {
	return AEAD_CHACHA20POLY1305
}

func (s chachaPolyScheme) New(key []byte) (cipher.AEAD, error) {
	return chacha20poly1305.New(key)
}

func (s chachaPolyScheme) KeySize() int {
	return chacha20poly1305.KeySize
}

func (s chachaPolyScheme) NonceSize() int {
	return chacha20poly1305.NonceSize
}

///////
// HKDF

type hkdfScheme struct {
	hash crypto.Hash
}

func (s hkdfScheme) ID() KDFID {
	switch s.hash {
	case crypto.SHA256:
		return KDF_HKDF_SHA256
	case crypto.SHA384:
		return KDF_HKDF_SHA384
	case crypto.SHA512:
		return KDF_HKDF_SHA512
	case crypto.SHA3_256:
		return KDF_HKDF_SHA3_256
	}
	panic(fmt.Sprintf("Unsupported hash: %d", s.hash))
}

func (s hkdfScheme) Hash(message []byte) []byte {
	h := s.hash.New()
	h.Write(message)
	return h.Sum(nil)
}

func (s hkdfScheme) Extract(salt, ikm []byte) []byte {
	saltOrZero := salt

	// if [salt is] not provided, it is set to a string of HashLen zeros
	if salt == nil {
		saltOrZero = make([]byte, s.hash.Size())
	}

	h := hmac.New(s.hash.New, saltOrZero)
	h.Write(ikm)
	return h.Sum(nil)
}

func (s hkdfScheme) Expand(prk, info []byte, outLen int) []byte {
	out := []byte{}
	T := []byte{}
	i := byte(1)
	for len(out) < outLen {
		block := append(T, info...)
		block = append(block, i)

		h := hmac.New(s.hash.New, prk)
		h.Write(block)

		T = h.Sum(nil)
		out = append(out, T...)
		i++
	}
	return out[:outLen]
}

func (s hkdfScheme) LabeledExtract(salt []byte, label string, ikm []byte) []byte {
	labeledIKM := append([]byte(rfcLabel+" "+label), ikm...)
	return s.Extract(salt, labeledIKM)
}

func (s hkdfScheme) LabeledExpand(prk []byte, label string, info []byte, L int) []byte {
	if L > (1 << 16) {
		panic("Expand length cannot be larger than 2^16")
	}

	lengthBuffer := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBuffer, uint16(L))
	labeledLength := append(lengthBuffer, []byte(rfcLabel+" "+label)...)
	labeledInfo := append(labeledLength, info...)
	return s.Expand(prk, labeledInfo, L)
}

func (s hkdfScheme) OutputSize() int {
	return s.hash.Size()
}

///////////////////////////
// Pre-defined KEM identifiers

type KEMID uint16

const (
	DHKEM_P256   KEMID = 0x0010
	DHKEM_P521   KEMID = 0x0012
	DHKEM_X25519 KEMID = 0x0020
	DHKEM_X448   KEMID = 0x0021
	KEM_SIKE503  KEMID = 0xFFFE
	KEM_SIKE751  KEMID = 0xFFFF
)

var kems = map[KEMID]KEMScheme{
	DHKEM_X25519: &dhkemScheme{group: x25519Scheme{}, KDF: hkdfScheme{hash: crypto.SHA256}},
	DHKEM_X448:   &dhkemScheme{group: x448Scheme{}, KDF: hkdfScheme{hash: crypto.SHA512}},
	DHKEM_P256:   &dhkemScheme{group: ecdhScheme{curve: elliptic.P256()}, KDF: hkdfScheme{hash: crypto.SHA256}},
	DHKEM_P521:   &dhkemScheme{group: ecdhScheme{curve: elliptic.P521()}, KDF: hkdfScheme{hash: crypto.SHA512}},
	KEM_SIKE503:  &sikeScheme{field: sidh.Fp503, KDF: hkdfScheme{hash: crypto.SHA512}},
	KEM_SIKE751:  &sikeScheme{field: sidh.Fp751, KDF: hkdfScheme{hash: crypto.SHA512}},
}

func newKEMScheme(kemID KEMID) (KEMScheme, bool) {
	switch kemID {
	case DHKEM_X25519:
		return &dhkemScheme{group: x25519Scheme{}, KDF: hkdfScheme{hash: crypto.SHA256}}, true
	case DHKEM_X448:
		return &dhkemScheme{group: x448Scheme{}, KDF: hkdfScheme{hash: crypto.SHA512}}, true
	case DHKEM_P256:
		return &dhkemScheme{group: ecdhScheme{curve: elliptic.P256()}, KDF: hkdfScheme{hash: crypto.SHA256}}, true
	case DHKEM_P521:
		return &dhkemScheme{group: ecdhScheme{curve: elliptic.P521()}, KDF: hkdfScheme{hash: crypto.SHA512}}, true
	case KEM_SIKE503:
		return &sikeScheme{field: sidh.Fp503, KDF: hkdfScheme{hash: crypto.SHA512}}, true
	case KEM_SIKE751:
		return &sikeScheme{field: sidh.Fp751, KDF: hkdfScheme{hash: crypto.SHA512}}, true
	default:
		return nil, false
	}
}

///////////////////////////
// Pre-defined KDF identifiers

type KDFID uint16

const (
	KDF_HKDF_SHA256   KDFID = 0x0001
	KDF_HKDF_SHA384   KDFID = 0x0002
	KDF_HKDF_SHA512   KDFID = 0x0003
	KDF_HKDF_SHA3_256 KDFID = 0x0004
)

var kdfs = map[KDFID]KDFScheme{
	KDF_HKDF_SHA256:   hkdfScheme{hash: crypto.SHA256},
	KDF_HKDF_SHA384:   hkdfScheme{hash: crypto.SHA384},
	KDF_HKDF_SHA512:   hkdfScheme{hash: crypto.SHA512},
	KDF_HKDF_SHA3_256: hkdfScheme{hash: crypto.SHA3_256},
}

///////////////////////////
// Pre-defined AEAD identifiers

type AEADID uint16

const (
	AEAD_AESGCM128        AEADID = 0x0001
	AEAD_AESGCM256        AEADID = 0x0002
	AEAD_CHACHA20POLY1305 AEADID = 0x0003
)

var aeads = map[AEADID]AEADScheme{
	AEAD_AESGCM128:        aesgcmScheme{keySize: 16},
	AEAD_AESGCM256:        aesgcmScheme{keySize: 32},
	AEAD_CHACHA20POLY1305: chachaPolyScheme{},
}

func AssembleCipherSuite(kemID KEMID, kdfID KDFID, aeadID AEADID) (CipherSuite, error) {
	kem, ok := newKEMScheme(kemID)
	if !ok {
		return CipherSuite{}, fmt.Errorf("Unknown KEM id")
	}

	kdf, ok := kdfs[kdfID]
	if !ok {
		return CipherSuite{}, fmt.Errorf("Unknown KDF id")
	}

	aead, ok := aeads[aeadID]
	if !ok {
		return CipherSuite{}, fmt.Errorf("Unknown AEAD id")
	}

	return CipherSuite{
		KEM:  kem,
		KDF:  kdf,
		AEAD: aead,
	}, nil
}

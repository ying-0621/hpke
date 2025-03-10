/*
import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	fixedPSK = []byte{0x02, 0x47, 0xFD, 0x33, 0xB9, 0x13, 0x76, 0x0F,
		0xA1, 0xFA, 0x51, 0xE1, 0x89, 0x2D, 0x9F, 0x30,
		0x7F, 0xBE, 0x65, 0xEB, 0x17, 0x1E, 0x81, 0x32,
		0xC2, 0xAF, 0x18, 0x55, 0x5A, 0x73, 0x8B, 0x82} // 32 bytes
	fixedPSKID    = []byte("Ennyn Durin aran Moria")
	original      = []byte("Beauty is truth, truth beauty")
	aad           = []byte("that is all // Ye know on earth, and all ye need to know")
	info          = []byte("Ode on a Grecian Urn")
	rtts          = 10
	exportContext = []byte("test export")
	exportLength  = 32
)

const (
	outputTestVectorEnvironmentKey = "HPKE_TEST_VECTORS_OUT"
	inputTestVectorEnvironmentKey  = "HPKE_TEST_VECTORS_IN"
	testVectorEncryptionCount      = 257
	testVectorExportLength         = 32
)

// /////
// Infallible Serialize / Deserialize
func fatalOnError(t *testing.T, err error, msg string) {
	realMsg := fmt.Sprintf("%s: %v", msg, err)
	if t != nil {
		require.Nil(t, err, realMsg)
	} else if err != nil {
		panic(realMsg)
	}
}

func mustUnhex(t *testing.T, h string) []byte {
	out, err := hex.DecodeString(h)
	fatalOnError(t, err, "Unhex failed")
	return out
}

func mustHex(d []byte) string {
	return hex.EncodeToString(d)
}

func mustDeserializePriv(t *testing.T, suite CipherSuite, h string, required bool) KEMPrivateKey {
	skm := mustUnhex(t, h)
	sk, err := suite.KEM.DeserializePrivateKey(skm)
	if required {
		fatalOnError(t, err, "DeserializePrivate failed")
	}
	return sk
}

func mustSerializePriv(suite CipherSuite, priv KEMPrivateKey) string {
	return mustHex(suite.KEM.SerializePrivateKey(priv))
}

func mustDeserializePub(t *testing.T, suite CipherSuite, h string, required bool) KEMPublicKey {
	pkm := mustUnhex(t, h)
	pk, err := suite.KEM.DeserializePublicKey(pkm)
	if required {
		fatalOnError(t, err, "DeserializePublicKey failed")
	}
	return pk
}

func mustSerializePub(suite CipherSuite, pub KEMPublicKey) string {
	return mustHex(suite.KEM.SerializePublicKey(pub))
}

func mustGenerateKeyPair(t *testing.T, suite CipherSuite) (KEMPrivateKey, KEMPublicKey, []byte) {
	ikm := make([]byte, suite.KEM.PrivateKeySize())
	rand.Reader.Read(ikm)
	sk, pk, err := suite.KEM.DeriveKeyPair(ikm)
	fatalOnError(t, err, "Error generating DH key pair")
	return sk, pk, ikm
}

// /////
// Assertions
func assert(t *testing.T, suite CipherSuite, msg string, test bool) {
	require.True(t, test, "[%04x, %04x, %04x] %s", suite.KEM.ID(), suite.KDF.ID(), suite.AEAD.ID(), msg)
}

func assertNotError(t *testing.T, suite CipherSuite, msg string, err error) {
	require.Nil(t, err, "%s: %v", msg, err)
}

func assertBytesEqual(t *testing.T, suite CipherSuite, msg string, lhs, rhs []byte) {
	// require.Equal treates a nil slice as different from a zero-length slice, so
	// we handle this as a special case.
	if len(lhs) == 0 && len(rhs) == 0 {
		return
	}

	require.Equal(t, lhs, rhs, "%s: [%x] != [%x]", msg, lhs, rhs)
}

func assertCipherContextEqual(t *testing.T, suite CipherSuite, msg string, lhs, rhs context) {
	// Verify the serialized fields match.
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "role"), lhs.Role == rhs.Role)
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "KEM id"), lhs.KEMID == rhs.KEMID)
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "KDF id"), lhs.KDFID == rhs.KDFID)
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "AEAD id"), lhs.AEADID == rhs.AEADID)
	assertBytesEqual(t, suite, fmt.Sprintf("%s: %s", msg, "exporter secret"), lhs.ExporterSecret, rhs.ExporterSecret)
	assertBytesEqual(t, suite, fmt.Sprintf("%s: %s", msg, "key"), lhs.Key, rhs.Key)
	assertBytesEqual(t, suite, fmt.Sprintf("%s: %s", msg, "base_nonce"), lhs.BaseNonce, rhs.BaseNonce)
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "sequence number"), lhs.Seq == rhs.Seq)

	// Verify that the internal representation of the cipher suite matches.
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "KEM scheme representation"), lhs.suite.KEM.ID() == rhs.suite.KEM.ID())
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "KDF scheme representation"), lhs.suite.KDF.ID() == rhs.suite.KDF.ID())
	assert(t, suite, fmt.Sprintf("%s: %s", msg, "AEAD scheme representation"), lhs.suite.AEAD.ID() == rhs.suite.AEAD.ID())

	if lhs.AEADID == AEAD_EXPORT_ONLY {
		return
	}

	// Verify that the internal AEAD object uses the same algorithm and is keyed
	// with the same key.
	var got, want []byte
	lhs.aead.Seal(got, lhs.BaseNonce, nil, nil)
	rhs.aead.Seal(want, rhs.BaseNonce, nil, nil)
	assertBytesEqual(t, suite, fmt.Sprintf("%s: %s", msg, "internal AEAD representation"), got, want)
}

// /////
// Symmetric encryption test vector structure
type encryptionTestVector struct {
	plaintext  []byte
	aad        []byte
	nonce      []byte
	ciphertext []byte
}

func (etv encryptionTestVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"pt":    mustHex(etv.plaintext),
		"aad":   mustHex(etv.aad),
		"nonce": mustHex(etv.nonce),
		"ct":    mustHex(etv.ciphertext),
	})
}

func (etv *encryptionTestVector) UnmarshalJSON(data []byte) error {
	raw := map[string]string{}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	etv.plaintext = mustUnhex(nil, raw["pt"])
	etv.aad = mustUnhex(nil, raw["aad"])
	etv.nonce = mustUnhex(nil, raw["nonce"])
	etv.ciphertext = mustUnhex(nil, raw["ct"])
	return nil
}

// /////
// Exporter test vector structures
type rawExporterTestVector struct {
	ExportContext string `json:"exporter_context"`
	ExportLength  int    `json:"L"`
	ExportValue   string `json:"exported_value"`
}

type exporterTestVector struct {
	exportContext []byte
	exportLength  int
	exportValue   []byte
}

func (etv exporterTestVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(rawExporterTestVector{
		ExportContext: mustHex(etv.exportContext),
		ExportLength:  etv.exportLength,
		ExportValue:   mustHex(etv.exportValue),
	})
}

func (etv *exporterTestVector) UnmarshalJSON(data []byte) error {
	raw := rawExporterTestVector{}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	etv.exportContext = mustUnhex(nil, raw.ExportContext)
	etv.exportLength = raw.ExportLength
	etv.exportValue = mustUnhex(nil, raw.ExportValue)
	return nil
}

// /////
// HPKE test vector structures
type rawTestVector struct {
	// Parameters
	Mode   Mode   `json:"mode"`
	KEMID  KEMID  `json:"kem_id"`
	KDFID  KDFID  `json:"kdf_id"`
	AEADID AEADID `json:"aead_id"`
	Info   string `json:"info"`

	// Private keys
	IKMR  string `json:"ikmR"`
	IKMS  string `json:"ikmS,omitempty"`
	IKME  string `json:"ikmE"`
	SKR   string `json:"skRm"`
	SKS   string `json:"skSm,omitempty"`
	SKE   string `json:"skEm"`
	PSK   string `json:"psk,omitempty"`
	PSKID string `json:"psk_id,omitempty"`

	// Public keys
	PKR string `json:"pkRm"`
	PKS string `json:"pkSm,omitempty"`
	PKE string `json:"pkEm"`

	// Key schedule inputs and computations
	Enc                string `json:"enc"`
	SharedSecret       string `json:"shared_secret"`
	KeyScheduleContext string `json:"key_schedule_context"`
	Secret             string `json:"secret"`
	Key                string `json:"key"`
	BaseNonce          string `json:"base_nonce"`
	ExporterSecret     string `json:"exporter_secret"`

	Encryptions []encryptionTestVector `json:"encryptions"`
	Exports     []exporterTestVector   `json:"exports"`
}

type testVector struct {
	t     *testing.T
	suite CipherSuite

	// Parameters
	mode    Mode
	kem_id  KEMID
	kdf_id  KDFID
	aead_id AEADID
	info    []byte

	// Private keys
	skR    KEMPrivateKey
	skS    KEMPrivateKey
	skE    KEMPrivateKey
	ikmR   []byte
	ikmS   []byte
	ikmE   []byte
	psk    []byte
	psk_id []byte

	// Public keys
	pkR KEMPublicKey
	pkS KEMPublicKey
	pkE KEMPublicKey

	// Key schedule inputs and computations
	enc                []byte
	sharedSecret       []byte
	keyScheduleContext []byte
	secret             []byte
	key                []byte
	baseNonce          []byte
	exporterSecret     []byte

	encryptions []encryptionTestVector
	exports     []exporterTestVector
}

func (tv testVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(rawTestVector{
		Mode:   tv.mode,
		KEMID:  tv.kem_id,
		KDFID:  tv.kdf_id,
		AEADID: tv.aead_id,
		Info:   mustHex(tv.info),

		IKMR:  mustHex(tv.ikmR),
		IKMS:  mustHex(tv.ikmS),
		IKME:  mustHex(tv.ikmE),
		SKR:   mustSerializePriv(tv.suite, tv.skR),
		SKS:   mustSerializePriv(tv.suite, tv.skS),
		SKE:   mustSerializePriv(tv.suite, tv.skE),
		PSK:   mustHex(tv.psk),
		PSKID: mustHex(tv.psk_id),

		PKR: mustSerializePub(tv.suite, tv.pkR),
		PKS: mustSerializePub(tv.suite, tv.pkS),
		PKE: mustSerializePub(tv.suite, tv.pkE),

		Enc:                mustHex(tv.enc),
		SharedSecret:       mustHex(tv.sharedSecret),
		KeyScheduleContext: mustHex(tv.keyScheduleContext),
		Secret:             mustHex(tv.secret),
		Key:                mustHex(tv.key),
		BaseNonce:          mustHex(tv.baseNonce),
		ExporterSecret:     mustHex(tv.exporterSecret),

		Encryptions: tv.encryptions,
		Exports:     tv.exports,
	})
}

func (tv *testVector) UnmarshalJSON(data []byte) error {
	raw := rawTestVector{}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	tv.mode = raw.Mode
	tv.kem_id = raw.KEMID
	tv.kdf_id = raw.KDFID
	tv.aead_id = raw.AEADID
	tv.info = mustUnhex(tv.t, raw.Info)

	tv.suite, err = AssembleCipherSuite(raw.KEMID, raw.KDFID, raw.AEADID)
	if err != nil {
		return err
	}

	modeRequiresSenderKey := (tv.mode == modeAuth || tv.mode == modeAuthPSK)
	tv.skR = mustDeserializePriv(tv.t, tv.suite, raw.SKR, true)
	tv.skS = mustDeserializePriv(tv.t, tv.suite, raw.SKS, modeRequiresSenderKey)
	tv.skE = mustDeserializePriv(tv.t, tv.suite, raw.SKE, true)

	tv.pkR = mustDeserializePub(tv.t, tv.suite, raw.PKR, true)
	tv.pkS = mustDeserializePub(tv.t, tv.suite, raw.PKS, modeRequiresSenderKey)
	tv.pkE = mustDeserializePub(tv.t, tv.suite, raw.PKE, true)

	tv.psk = mustUnhex(tv.t, raw.PSK)
	tv.psk_id = mustUnhex(tv.t, raw.PSKID)

	tv.ikmR = mustUnhex(tv.t, raw.IKMR)
	tv.ikmS = mustUnhex(tv.t, raw.IKMS)
	tv.ikmE = mustUnhex(tv.t, raw.IKME)

	tv.enc = mustUnhex(tv.t, raw.Enc)
	tv.sharedSecret = mustUnhex(tv.t, raw.SharedSecret)
	tv.keyScheduleContext = mustUnhex(tv.t, raw.KeyScheduleContext)
	tv.secret = mustUnhex(tv.t, raw.Secret)
	tv.key = mustUnhex(tv.t, raw.Key)
	tv.baseNonce = mustUnhex(tv.t, raw.BaseNonce)
	tv.exporterSecret = mustUnhex(tv.t, raw.ExporterSecret)

	tv.encryptions = raw.Encryptions
	tv.exports = raw.Exports
	return nil
}

type testVectorArray struct {
	t       *testing.T
	vectors []testVector
}

func (tva testVectorArray) MarshalJSON() ([]byte, error) {
	return json.Marshal(tva.vectors)
}

func (tva *testVectorArray) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &tva.vectors)
	if err != nil {
		return err
	}

	for i := range tva.vectors {
		tva.vectors[i].t = tva.t
	}
	return nil
}

// /////
// Generalize setup functions so that we can iterate over them easily
type setupMode struct {
	Mode Mode
	OK   func(suite CipherSuite) bool
	I    func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, psk_id []byte) ([]byte, *SenderContext, error)
	R    func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, psk_id []byte) (*ReceiverContext, error)
}

var setupModes = map[Mode]setupMode{
	modeBase: {
		Mode: modeBase,
		OK:   func(suite CipherSuite) bool { return true },
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, psk_id []byte) ([]byte, *SenderContext, error) {
			return SetupBaseS(suite, rand.Reader, pkR, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, psk_id []byte) (*ReceiverContext, error) {
			return SetupBaseR(suite, skR, enc, info)
		},
	},
	modePSK: {
		Mode: modePSK,
		OK:   func(suite CipherSuite) bool { return true },
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, psk_id []byte) ([]byte, *SenderContext, error) {
			return SetupPSKS(suite, rand.Reader, pkR, psk, psk_id, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, psk_id []byte) (*ReceiverContext, error) {
			return SetupPSKR(suite, skR, enc, psk, psk_id, info)
		},
	},
	modeAuth: {
		Mode: modeAuth,
		OK: func(suite CipherSuite) bool {
			_, ok := suite.KEM.(AuthKEMScheme)
			return ok
		},
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, psk_id []byte) ([]byte, *SenderContext, error) {
			return SetupAuthS(suite, rand.Reader, pkR, skS, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, psk_id []byte) (*ReceiverContext, error) {
			return SetupAuthR(suite, skR, pkS, enc, info)
		},
	},
	modeAuthPSK: {
		Mode: modeAuthPSK,
		OK: func(suite CipherSuite) bool {
			_, ok := suite.KEM.(AuthKEMScheme)
			return ok
		},
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, psk_id []byte) ([]byte, *SenderContext, error) {
			return SetupAuthPSKS(suite, rand.Reader, pkR, skS, psk, psk_id, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, psk_id []byte) (*ReceiverContext, error) {
			return SetupAuthPSKR(suite, skR, pkS, enc, psk, psk_id, info)
		},
	},
}

///////
// Direct tests

type roundTripTest struct {
	kem_id  KEMID
	kdf_id  KDFID
	aead_id AEADID
	setup   setupMode
}

func (rtt roundTripTest) Test(t *testing.T) {
	suite, err := AssembleCipherSuite(rtt.kem_id, rtt.kdf_id, rtt.aead_id)
	require.Nil(t, err, "[%04x, %04x, %04x] Error looking up ciphersuite: %v", rtt.kem_id, rtt.kdf_id, rtt.aead_id, err)

	if !rtt.setup.OK(suite) {
		return
	}

	skS, pkS, _ := mustGenerateKeyPair(t, suite)
	skR, pkR, _ := mustGenerateKeyPair(t, suite)

	enc, ctxS, err := rtt.setup.I(suite, pkR, info, skS, fixedPSK, fixedPSKID)
	assertNotError(t, suite, "Error in SetupI", err)

	ctxR, err := rtt.setup.R(suite, skR, enc, info, pkS, fixedPSK, fixedPSKID)
	assertNotError(t, suite, "Error in SetupR", err)

	// Verify encryption functionality, if applicable
	if rtt.aead_id != AEAD_EXPORT_ONLY {
		for range make([]struct{}, rtts) {
			encrypted := ctxS.Seal(aad, original)
			decrypted, err := ctxR.Open(aad, encrypted)
			assertNotError(t, suite, "Error in Open", err)
			assertBytesEqual(t, suite, "Incorrect decryption", decrypted, original)
		}
	}

	// Verify exporter functionality
	exportedI := ctxS.Export(exportContext, exportLength)
	exportedR := ctxR.Export(exportContext, exportLength)
	assertBytesEqual(t, suite, "Incorrect exported secret", exportedI, exportedR)

	// Verify encryption context serialization functionality
	opaqueI, err := ctxS.Marshal()
	assertNotError(t, suite, "Error serializing encrypt context", err)

	unmarshaledI, err := UnmarshalSenderContext(opaqueI)
	assertNotError(t, suite, "Error deserializing encrypt context", err)
	assertCipherContextEqual(t, suite, "Encrypt context serialization mismatch", ctxS.context, unmarshaledI.context)

	// Verify decryption context serialization functionality
	opaqueR, err := ctxR.Marshal()
	assertNotError(t, suite, "Error serializing decrypt context", err)

	unmarshaledR, err := UnmarshalReceiverContext(opaqueR)
	assertNotError(t, suite, "Error deserializing decrypt context", err)
	assertCipherContextEqual(t, suite, "Decrypt context serialization mismatch", ctxR.context, unmarshaledR.context)

	// Verify exporter functionality for a deserialized context
	assertBytesEqual(t, suite, "Export after serialization fails for sender", exportedI, unmarshaledI.Export(exportContext, exportLength))
	assertBytesEqual(t, suite, "Export after serialization fails for receiver", exportedR, unmarshaledR.Export(exportContext, exportLength))
}

func TestModes(t *testing.T) {
	for kem_id, _ := range kems {
		for kdf_id, _ := range kdfs {
			for aead_id, _ := range aeads {
				for mode, setup := range setupModes {
					label := fmt.Sprintf("kem=%04x/kdf=%04x/aead=%04x/mode=%02x", kem_id, kdf_id, aead_id, mode)
					rtt := roundTripTest{kem_id, kdf_id, aead_id, setup}
					t.Run(label, rtt.Test)
				}
			}
		}
	}
}

///////
// Generation and processing of test vectors

func verifyEncryptions(tv testVector, enc *SenderContext, dec *ReceiverContext) {
	for _, data := range tv.encryptions {
		encrypted := enc.Seal(data.aad, data.plaintext)
		decrypted, err := dec.Open(data.aad, encrypted)

		assertNotError(tv.t, tv.suite, "Error in Open", err)
		assertBytesEqual(tv.t, tv.suite, "Incorrect encryption", encrypted, data.ciphertext)
		assertBytesEqual(tv.t, tv.suite, "Incorrect decryption", decrypted, data.plaintext)
	}
}

func verifyParameters(tv testVector, ctx context) {
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'shared_secret'", tv.sharedSecret, ctx.setupParams.sharedSecret)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'enc'", tv.enc, ctx.setupParams.enc)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'key_schedule_context'", tv.keyScheduleContext, ctx.contextParams.keyScheduleContext)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'secret'", tv.secret, ctx.contextParams.secret)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'key'", tv.key, ctx.Key)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'base_nonce'", tv.baseNonce, ctx.BaseNonce)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'exporter_secret'", tv.exporterSecret, ctx.ExporterSecret)
}

func verifyPublicKeysEqual(tv testVector, pkX, pkY KEMPublicKey) {
	pkXm := mustSerializePub(tv.suite, pkX)
	pkYm := mustSerializePub(tv.suite, pkY)
	assertBytesEqual(tv.t, tv.suite, "Incorrect public key", []byte(pkXm), []byte(pkYm))
}

func verifyPrivateKeysEqual(tv testVector, skX, skY KEMPrivateKey) {
	skXm := mustSerializePriv(tv.suite, skX)
	skYm := mustSerializePriv(tv.suite, skY)
	assertBytesEqual(tv.t, tv.suite, "Incorrect private key", []byte(skXm), []byte(skYm))
}

func verifyTestVector(tv testVector) {
	setup := setupModes[tv.mode]

	skR, pkR, err := tv.suite.KEM.DeriveKeyPair(tv.ikmR)
	assertNotError(tv.t, tv.suite, "Error in DeriveKeyPair", err)
	verifyPublicKeysEqual(tv, tv.pkR, pkR)
	verifyPrivateKeysEqual(tv, tv.skR, skR)

	skE, pkE, err := tv.suite.KEM.DeriveKeyPair(tv.ikmE)
	assertNotError(tv.t, tv.suite, "Error in DeriveKeyPair", err)
	verifyPublicKeysEqual(tv, tv.pkE, pkE)
	verifyPrivateKeysEqual(tv, tv.skE, skE)

	tv.suite.KEM.setEphemeralKeyPair(skE)

	var pkS KEMPublicKey
	var skS KEMPrivateKey
	if setup.Mode == modeAuth || setup.Mode == modeAuthPSK {
		skS, pkS, err = tv.suite.KEM.DeriveKeyPair(tv.ikmS)
		assertNotError(tv.t, tv.suite, "Error in DeriveKeyPair", err)
		verifyPublicKeysEqual(tv, tv.pkS, pkS)
		verifyPrivateKeysEqual(tv, tv.skS, skS)
	}

	enc, ctxS, err := setup.I(tv.suite, pkR, tv.info, skS, tv.psk, tv.psk_id)
	assertNotError(tv.t, tv.suite, "Error in SetupI", err)
	assertBytesEqual(tv.t, tv.suite, "Encapsulated key mismatch", enc, tv.enc)

	ctxR, err := setup.R(tv.suite, skR, tv.enc, tv.info, pkS, tv.psk, tv.psk_id)
	assertNotError(tv.t, tv.suite, "Error in SetupR", err)

	verifyParameters(tv, ctxS.context)
	verifyParameters(tv, ctxR.context)

	verifyEncryptions(tv, ctxS, ctxR)
}

func vectorTest(vector testVector) func(t *testing.T) {
	return func(t *testing.T) {
		verifyTestVector(vector)
	}
}

func verifyTestVectors(t *testing.T, vectorString []byte, subtest bool) {
	vectors := testVectorArray{t: t}
	err := json.Unmarshal(vectorString, &vectors)
	require.Nil(t, err, "Error decoding test vector string: %v", err)

	for _, tv := range vectors.vectors {
		test := vectorTest(tv)
		if !subtest {
			test(t)
		} else {
			label := fmt.Sprintf("kem=%04x/kdf=%04x/aead=%04x/mode=%02x", tv.kem_id, tv.kdf_id, tv.aead_id, tv.mode)
			t.Run(label, test)
		}
	}
}

func generateEncryptions(t *testing.T, suite CipherSuite, ctxS *SenderContext, ctxR *ReceiverContext) ([]encryptionTestVector, error) {
	vectors := make([]encryptionTestVector, testVectorEncryptionCount)
	for i := 0; i < len(vectors); i++ {
		aad := []byte(fmt.Sprintf("Count-%d", i))
		encrypted := ctxS.Seal(aad, original)
		decrypted, err := ctxR.Open(aad, encrypted)
		assertNotError(t, suite, "Decryption failure", err)
		assertBytesEqual(t, suite, "Incorrect decryption", original, decrypted)

		vectors[i] = encryptionTestVector{
			plaintext:  original,
			aad:        aad,
			nonce:      ctxS.nonces[i],
			ciphertext: encrypted,
		}
	}

	return vectors, nil
}

func generateExports(t *testing.T, suite CipherSuite, ctxS *SenderContext, ctxR *ReceiverContext) ([]exporterTestVector, error) {
	exportContexts := [][]byte{
		[]byte(""),
		[]byte{0x00},
		[]byte("TestContext"),
	}
	vectors := make([]exporterTestVector, len(exportContexts))
	for i := 0; i < len(vectors); i++ {
		exportI := ctxS.Export(exportContexts[i], testVectorExportLength)
		exportR := ctxR.Export(exportContexts[i], testVectorExportLength)
		assertBytesEqual(t, suite, "Incorrect export", exportI, exportR)
		vectors[i] = exporterTestVector{
			exportContext: exportContexts[i],
			exportLength:  testVectorExportLength,
			exportValue:   exportI,
		}
	}

	return vectors, nil
}

func generateTestVector(t *testing.T, setup setupMode, kem_id KEMID, kdf_id KDFID, aead_id AEADID) testVector {
	suite, err := AssembleCipherSuite(kem_id, kdf_id, aead_id)
	require.Nil(t, err, "[%x, %x, %x] Error looking up ciphersuite: %s", kem_id, kdf_id, aead_id, err)

	skR, pkR, ikmR := mustGenerateKeyPair(t, suite)
	skE, pkE, ikmE := mustGenerateKeyPair(t, suite)

	// The sender key share is only required for Auth mode variants.
	var pkS KEMPublicKey
	var skS KEMPrivateKey
	var ikmS []byte
	if setup.Mode == modeAuth || setup.Mode == modeAuthPSK {
		skS, pkS, ikmS = mustGenerateKeyPair(t, suite)
	}

	// A PSK is only required for PSK mode variants.
	var psk []byte
	var psk_id []byte
	if setup.Mode == modePSK || setup.Mode == modeAuthPSK {
		psk = fixedPSK
		psk_id = fixedPSKID
	}

	suite.KEM.setEphemeralKeyPair(skE)

	enc, ctxS, err := setup.I(suite, pkR, info, skS, psk, psk_id)
	assertNotError(t, suite, "Error in SetupPSKS", err)

	ctxR, err := setup.R(suite, skR, enc, info, pkS, psk, psk_id)
	assertNotError(t, suite, "Error in SetupPSKR", err)

	encryptionVectors := []encryptionTestVector{}
	if aead_id != AEAD_EXPORT_ONLY {
		encryptionVectors, err = generateEncryptions(t, suite, ctxS, ctxR)
		assertNotError(t, suite, "Error in generateEncryptions", err)
	}

	exportVectors, err := generateExports(t, suite, ctxS, ctxR)
	assertNotError(t, suite, "Error in generateExports", err)

	vector := testVector{
		t:                  t,
		suite:              suite,
		mode:               setup.Mode,
		kem_id:             kem_id,
		kdf_id:             kdf_id,
		aead_id:            aead_id,
		info:               info,
		skR:                skR,
		pkR:                pkR,
		skS:                skS,
		pkS:                pkS,
		skE:                skE,
		pkE:                pkE,
		ikmR:               ikmR,
		ikmS:               ikmS,
		ikmE:               ikmE,
		psk:                psk,
		psk_id:             psk_id,
		enc:                ctxS.setupParams.enc,
		sharedSecret:       ctxS.setupParams.sharedSecret,
		keyScheduleContext: ctxS.contextParams.keyScheduleContext,
		secret:             ctxS.contextParams.secret,
		key:                ctxS.Key,
		baseNonce:          ctxS.BaseNonce,
		exporterSecret:     ctxS.ExporterSecret,
		encryptions:        encryptionVectors,
		exports:            exportVectors,
	}

	return vector
}

func TestVectorGenerate(t *testing.T) {
	// We only generate test vectors for select ciphersuites
	supportedKEMs := []KEMID{DHKEM_X25519, DHKEM_X448, DHKEM_P256, DHKEM_P521}
	supportedKDFs := []KDFID{KDF_HKDF_SHA256, KDF_HKDF_SHA512}
	supportedAEADs := []AEADID{AEAD_AESGCM128, AEAD_AESGCM256, AEAD_CHACHA20POLY1305, AEAD_EXPORT_ONLY}

	vectors := make([]testVector, 0)
	for _, kem_id := range supportedKEMs {
		for _, kdf_id := range supportedKDFs {
			for _, aead_id := range supportedAEADs {
				for _, setup := range setupModes {
					vectors = append(vectors, generateTestVector(t, setup, kem_id, kdf_id, aead_id))
				}
			}
		}
	}

	// Encode the test vectors
	encoded, err := json.Marshal(vectors)
	require.Nil(t, err, "Error producing test vectors: %v", err)

	// Verify that we process them correctly
	verifyTestVectors(t, encoded, false)

	// Write them to a file if requested
	var outputFile string
	if outputFile = os.Getenv(outputTestVectorEnvironmentKey); len(outputFile) > 0 {
		err = ioutil.WriteFile(outputFile, encoded, 0644)
		require.Nil(t, err, "Error writing test vectors: %v", err)
	}
}

func TestVectorVerify(t *testing.T) {
	var inputFile string
	if inputFile = os.Getenv(inputTestVectorEnvironmentKey); len(inputFile) == 0 {
		t.Skip("Test vectors were not provided")
	}

	encoded, err := ioutil.ReadFile(inputFile)
	require.Nil(t, err, "Failed reading test vectors: %v", err)

	verifyTestVectors(t, encoded, true)
}
*/

package hpke

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

var (
	fixedPSK      = []byte{0x5d, 0xb3, 0xb8, 0x0a, 0x81, 0xcb, 0x63, 0xca, 0x59, 0x47, 0x0c, 0x83, 0x41, 0x4e, 0xf7, 0x0a}
	fixedPSKID    = []byte("Ennyn Durin aran Moria")
	original      = []byte("Beauty is truth, truth beauty")
	aad           = []byte("that is all // Ye know on earth, and all ye need to know")
	info          = []byte("Ode on a Grecian Urn")
	rtts          = 10
	exportContext = []byte("test export")
	exportLength  = 32
)

const (
	outputTestVectorEnvironmentKey = "HPKE_TEST_VECTORS_OUT"
	inputTestVectorEnvironmentKey  = "HPKE_TEST_VECTORS_IN"
	testVectorEncryptionCount      = 10
	testVectorExportCount          = 5
	testVectorExportLength         = 32
)

// /////
// Infallible marshal / unmarshal
func fatalOnError(t *testing.T, err error, msg string) {
	realMsg := fmt.Sprintf("%s: %v", msg, err)
	if err != nil {
		if t != nil {
			//t.Fatalf(realMsg)
		} else {
			panic(realMsg)
		}
	}
}

func mustUnhex(t *testing.T, h string) []byte {
	out, err := hex.DecodeString(h)
	fatalOnError(t, err, "Unhex failed")
	return out
}

func mustHex(d []byte) string {
	return hex.EncodeToString(d)
}

func mustUnmarshalPriv(t *testing.T, suite CipherSuite, h string, required bool) KEMPrivateKey {
	skm := mustUnhex(t, h)
	sk, err := suite.KEM.UnmarshalPrivate(skm)
	if required {
		fatalOnError(t, err, "unmarshalPrivate failed")
	}
	return sk
}

func mustMarshalPriv(suite CipherSuite, priv KEMPrivateKey) string {
	return mustHex(suite.KEM.MarshalPrivate(priv))
}

func mustUnmarshalPub(t *testing.T, suite CipherSuite, h string, required bool) KEMPublicKey {
	pkm := mustUnhex(t, h)
	pk, err := suite.KEM.Unmarshal(pkm)
	if required {
		fatalOnError(t, err, "Unmarshal failed")
	}
	return pk
}

func mustMarshalPub(suite CipherSuite, pub KEMPublicKey) string {
	return mustHex(suite.KEM.Marshal(pub))
}

func mustGenerateKeyPair(t *testing.T, suite CipherSuite) (KEMPrivateKey, KEMPublicKey) {
	sk, pk, err := suite.KEM.GenerateKeyPair(rand.Reader)
	fatalOnError(t, err, "Error generating DH key pair")
	return sk, pk
}

// /////
// Assertions
func assert(t *testing.T, suite CipherSuite, msg string, test bool) {
	if !test {
		t.Fatalf("[%04x, %04x, %04x] %s", suite.KEM.ID(), suite.KDF.ID(), suite.AEAD.ID(), msg)
	}
}

func assertNotError(t *testing.T, suite CipherSuite, msg string, err error) {
	realMsg := fmt.Sprintf("%s: %v", msg, err)
	assert(t, suite, realMsg, err == nil)
}

func assertBytesEqual(t *testing.T, suite CipherSuite, msg string, lhs, rhs []byte) {
	realMsg := fmt.Sprintf("%s: [%x] != [%x]", msg, lhs, rhs)
	assert(t, suite, realMsg, bytes.Equal(lhs, rhs))
}

// /////
// Symmetric encryption test vector structure
type encryptionTestVector struct {
	plaintext  []byte
	aad        []byte
	nonce      []byte
	ciphertext []byte
}

func (etv encryptionTestVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"plaintext":  mustHex(etv.plaintext),
		"aad":        mustHex(etv.aad),
		"nonce":      mustHex(etv.nonce),
		"ciphertext": mustHex(etv.ciphertext),
	})
}

func (etv *encryptionTestVector) UnmarshalJSON(data []byte) error {
	raw := map[string]string{}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	etv.plaintext = mustUnhex(nil, raw["plaintext"])
	etv.aad = mustUnhex(nil, raw["aad"])
	etv.nonce = mustUnhex(nil, raw["nonce"])
	etv.ciphertext = mustUnhex(nil, raw["ciphertext"])
	return nil
}

// /////
// Exporter test vector structures
type rawExporterTestVector struct {
	ExportContext string `json:"exportContext"`
	ExportLength  int    `json:"exportLength"`
	ExportValue   string `json:"exportValue"`
}

type exporterTestVector struct {
	exportContext []byte
	exportLength  int
	exportValue   []byte
}

func (etv exporterTestVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(rawExporterTestVector{
		ExportContext: mustHex(etv.exportContext),
		ExportLength:  etv.exportLength,
		ExportValue:   mustHex(etv.exportValue),
	})
}

func (etv *exporterTestVector) UnmarshalJSON(data []byte) error {
	raw := rawExporterTestVector{}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	etv.exportContext = mustUnhex(nil, raw.ExportContext)
	etv.exportLength = raw.ExportLength
	etv.exportValue = mustUnhex(nil, raw.ExportValue)
	return nil
}

// /////
// HPKE test vector structures
type rawTestVector struct {
	// Parameters
	Mode   HPKEMode `json:"mode"`
	KEMID  KEMID    `json:"kemID"`
	KDFID  KDFID    `json:"kdfID"`
	AEADID AEADID   `json:"aeadID"`
	Info   string   `json:"info"`

	// Private keys
	SKR   string `json:"skRm"`
	SKS   string `json:"skSm,omitempty"`
	SKE   string `json:"skEm"`
	PSK   string `json:"psk,omitempty"`
	PSKID string `json:"pskID,omitempty"`

	// Public keys
	PKR string `json:"pkRm"`
	PKI string `json:"pkSm,omitempty"`
	PKE string `json:"pkEm"`

	// Key schedule inputs and computations
	Enc                string `json:"enc"`
	Zz                 string `json:"zz"`
	KeyScheduleContext string `json:"key_schedule_context"`
	Secret             string `json:"secret"`
	Key                string `json:"key"`
	Nonce              string `json:"nonce"`
	ExporterSecret     string `json:"exporterSecret"`

	Encryptions []encryptionTestVector `json:"encryptions"`
	Exports     []exporterTestVector   `json:"exports"`
}

type testVector struct {
	t     *testing.T
	suite CipherSuite

	// Parameters
	mode   HPKEMode
	kemID  KEMID
	kdfID  KDFID
	aeadID AEADID
	info   []byte

	// Private keys
	skR   KEMPrivateKey
	skS   KEMPrivateKey
	skE   KEMPrivateKey
	psk   []byte
	pskID []byte

	// Public keys
	pkR KEMPublicKey
	pkS KEMPublicKey
	pkE KEMPublicKey

	// Key schedule inputs and computations
	enc                []byte
	zz                 []byte
	keyScheduleContext []byte
	secret             []byte
	key                []byte
	nonce              []byte
	exporterSecret     []byte

	encryptions []encryptionTestVector
	exports     []exporterTestVector
}

func (tv testVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(rawTestVector{
		Mode:   tv.mode,
		KEMID:  tv.kemID,
		KDFID:  tv.kdfID,
		AEADID: tv.aeadID,
		Info:   mustHex(tv.info),

		SKR:   mustMarshalPriv(tv.suite, tv.skR),
		SKS:   mustMarshalPriv(tv.suite, tv.skS),
		SKE:   mustMarshalPriv(tv.suite, tv.skE),
		PSK:   mustHex(tv.psk),
		PSKID: mustHex(tv.pskID),

		PKR: mustMarshalPub(tv.suite, tv.pkR),
		PKI: mustMarshalPub(tv.suite, tv.pkS),
		PKE: mustMarshalPub(tv.suite, tv.pkE),

		Enc:                mustHex(tv.enc),
		Zz:                 mustHex(tv.zz),
		KeyScheduleContext: mustHex(tv.keyScheduleContext),
		Secret:             mustHex(tv.secret),
		Key:                mustHex(tv.key),
		Nonce:              mustHex(tv.nonce),
		ExporterSecret:     mustHex(tv.exporterSecret),

		Encryptions: tv.encryptions,
		Exports:     tv.exports,
	})
}

func (tv *testVector) UnmarshalJSON(data []byte) error {
	raw := rawTestVector{}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	tv.mode = raw.Mode
	tv.kemID = raw.KEMID
	tv.kdfID = raw.KDFID
	tv.aeadID = raw.AEADID
	tv.info = mustUnhex(tv.t, raw.Info)

	tv.suite, err = AssembleCipherSuite(raw.KEMID, raw.KDFID, raw.AEADID)
	if err != nil {
		return err
	}

	modeRequiresSenderKey := (tv.mode == modeAuth || tv.mode == modeAuthPSK)
	tv.skR = mustUnmarshalPriv(tv.t, tv.suite, raw.SKR, true)
	tv.skS = mustUnmarshalPriv(tv.t, tv.suite, raw.SKS, modeRequiresSenderKey)
	tv.skE = mustUnmarshalPriv(tv.t, tv.suite, raw.SKE, true)
	tv.psk = mustUnhex(tv.t, raw.PSK)
	tv.pskID = mustUnhex(tv.t, raw.PSKID)

	tv.suite.KEM.setEphemeralKeyPair(tv.skE)

	tv.pkR = mustUnmarshalPub(tv.t, tv.suite, raw.PKR, true)
	tv.pkS = mustUnmarshalPub(tv.t, tv.suite, raw.PKI, modeRequiresSenderKey)
	tv.pkE = mustUnmarshalPub(tv.t, tv.suite, raw.PKE, true)

	tv.enc = mustUnhex(tv.t, raw.Enc)
	tv.zz = mustUnhex(tv.t, raw.Zz)
	tv.keyScheduleContext = mustUnhex(tv.t, raw.KeyScheduleContext)
	tv.secret = mustUnhex(tv.t, raw.Secret)
	tv.key = mustUnhex(tv.t, raw.Key)
	tv.nonce = mustUnhex(tv.t, raw.Nonce)
	tv.exporterSecret = mustUnhex(tv.t, raw.ExporterSecret)

	tv.encryptions = raw.Encryptions
	tv.exports = raw.Exports
	return nil
}

type testVectorArray struct {
	t       *testing.T
	vectors []testVector
}

func (tva testVectorArray) MarshalJSON() ([]byte, error) {
	return json.Marshal(tva.vectors)
}

func (tva *testVectorArray) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &tva.vectors)
	if err != nil {
		return err
	}

	for i := range tva.vectors {
		tva.vectors[i].t = tva.t
	}
	return nil
}

// /////
// Generalize setup functions so that we can iterate over them easily
type setupMode struct {
	Mode HPKEMode
	OK   func(suite CipherSuite) bool
	I    func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, pskID []byte) ([]byte, *EncryptContext, error)
	R    func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, pskID []byte) (*DecryptContext, error)
}

var setupModes = map[HPKEMode]setupMode{
	modeBase: {
		Mode: modeBase,
		OK:   func(suite CipherSuite) bool { return true },
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, pskID []byte) ([]byte, *EncryptContext, error) {
			return SetupBaseS(suite, rand.Reader, pkR, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, pskID []byte) (*DecryptContext, error) {
			return SetupBaseR(suite, skR, enc, info)
		},
	},
	modePSK: {
		Mode: modePSK,
		OK:   func(suite CipherSuite) bool { return true },
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, pskID []byte) ([]byte, *EncryptContext, error) {
			return SetupPSKS(suite, rand.Reader, pkR, psk, pskID, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, pskID []byte) (*DecryptContext, error) {
			return SetupPSKR(suite, skR, enc, psk, pskID, info)
		},
	},
	modeAuth: {
		Mode: modeAuth,
		OK: func(suite CipherSuite) bool {
			_, ok := suite.KEM.(AuthKEMScheme)
			return ok
		},
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, pskID []byte) ([]byte, *EncryptContext, error) {
			return SetupAuthS(suite, rand.Reader, pkR, skS, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, pskID []byte) (*DecryptContext, error) {
			return SetupAuthR(suite, skR, pkS, enc, info)
		},
	},
	modeAuthPSK: {
		Mode: modeAuthPSK,
		OK: func(suite CipherSuite) bool {
			_, ok := suite.KEM.(AuthKEMScheme)
			return ok
		},
		I: func(suite CipherSuite, pkR KEMPublicKey, info []byte, skS KEMPrivateKey, psk, pskID []byte) ([]byte, *EncryptContext, error) {
			return SetupAuthPSKS(suite, rand.Reader, pkR, skS, psk, pskID, info)
		},
		R: func(suite CipherSuite, skR KEMPrivateKey, enc, info []byte, pkS KEMPublicKey, psk, pskID []byte) (*DecryptContext, error) {
			return SetupAuthPSKR(suite, skR, pkS, enc, psk, pskID, info)
		},
	},
}

///////
// Direct tests

type roundTripTest struct {
	kemID  KEMID
	kdfID  KDFID
	aeadID AEADID
	setup  setupMode
}

func (rtt roundTripTest) Test(t *testing.T) {
	suite, err := AssembleCipherSuite(rtt.kemID, rtt.kdfID, rtt.aeadID)
	if err != nil {
		t.Fatalf("[%04x, %04x, %04x] Error looking up ciphersuite: %v", rtt.kemID, rtt.kdfID, rtt.aeadID, err)
	}

	if !rtt.setup.OK(suite) {
		return
	}

	skS, pkS := mustGenerateKeyPair(t, suite)
	skR, pkR := mustGenerateKeyPair(t, suite)

	enc, ctxI, err := rtt.setup.I(suite, pkR, info, skS, fixedPSK, fixedPSKID)
	assertNotError(t, suite, "Error in SetupI", err)

	ctxR, err := rtt.setup.R(suite, skR, enc, info, pkS, fixedPSK, fixedPSKID)
	assertNotError(t, suite, "Error in SetupR", err)

	// Verify encryption functionality
	for range make([]struct{}, rtts) {
		encrypted := ctxI.Seal(aad, original)
		decrypted, err := ctxR.Open(aad, encrypted)
		assertNotError(t, suite, "Error in Open", err)
		assertBytesEqual(t, suite, "Incorrect decryption", decrypted, original)
	}

	// Verify exporter functionality
	exportedI := ctxI.Export(exportContext, exportLength)
	exportedR := ctxR.Export(exportContext, exportLength)
	assertBytesEqual(t, suite, "Incorrect exported secret", exportedI, exportedR)
}

func TestModes(t *testing.T) {
	for kemID, _ := range kems {
		for kdfID, _ := range kdfs {
			for aeadID, _ := range aeads {
				for mode, setup := range setupModes {
					label := fmt.Sprintf("kem=%04x/kdf=%04x/aead=%04x/mode=%02x", kemID, kdfID, aeadID, mode)
					rtt := roundTripTest{kemID, kdfID, aeadID, setup}
					t.Run(label, rtt.Test)
				}
			}
		}
	}
}

///////
// Generation and processing of test vectors

func verifyEncryptions(tv testVector, enc *EncryptContext, dec *DecryptContext) {
	for _, data := range tv.encryptions {
		encrypted := enc.Seal(data.aad, data.plaintext)
		decrypted, err := dec.Open(data.aad, encrypted)

		assertNotError(tv.t, tv.suite, "Error in Open", err)
		assertBytesEqual(tv.t, tv.suite, "Incorrect encryption", encrypted, data.ciphertext)
		assertBytesEqual(tv.t, tv.suite, "Incorrect decryption", decrypted, data.plaintext)
	}
}

func verifyParameters(tv testVector, ctx cipherContext) {
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'zz'", tv.zz, ctx.setupParams.zz)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'enc'", tv.enc, ctx.setupParams.enc)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'key_schedule_context'", tv.keyScheduleContext, ctx.contextParams.keyScheduleContext)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'secret'", tv.secret, ctx.contextParams.secret)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'key'", tv.key, ctx.key)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'nonce'", tv.nonce, ctx.nonce)
	assertBytesEqual(tv.t, tv.suite, "Incorrect parameter 'exporterSecret'", tv.exporterSecret, ctx.exporterSecret)
}

func verifyTestVector(tv testVector) {
	setup := setupModes[tv.mode]

	enc, ctxI, err := setup.I(tv.suite, tv.pkR, tv.info, tv.skS, tv.psk, tv.pskID)
	assertNotError(tv.t, tv.suite, "Error in SetupI", err)
	assertBytesEqual(tv.t, tv.suite, "Encapsulated key mismatch", enc, tv.enc)

	ctxR, err := setup.R(tv.suite, tv.skR, tv.enc, tv.info, tv.pkS, tv.psk, tv.pskID)
	assertNotError(tv.t, tv.suite, "Error in SetupR", err)

	verifyParameters(tv, ctxI.cipherContext)
	verifyParameters(tv, ctxR.cipherContext)

	verifyEncryptions(tv, ctxI, ctxR)
}

func vectorTest(vector testVector) func(t *testing.T) {
	return func(t *testing.T) {
		verifyTestVector(vector)
	}
}

func verifyTestVectors(t *testing.T, vectorString []byte, subtest bool) {
	vectors := testVectorArray{t: t}
	err := json.Unmarshal(vectorString, &vectors)
	if err != nil {
		t.Fatalf("Error decoding test vector string: %v", err)
	}

	for _, tv := range vectors.vectors {
		test := vectorTest(tv)
		if !subtest {
			test(t)
		} else {
			label := fmt.Sprintf("kem=%04x/kdf=%04x/aead=%04x/mode=%02x", tv.kemID, tv.kdfID, tv.aeadID, tv.mode)
			t.Run(label, test)
		}
	}
}

func generateEncryptions(t *testing.T, suite CipherSuite, ctxI *EncryptContext, ctxR *DecryptContext) ([]encryptionTestVector, error) {
	vectors := make([]encryptionTestVector, testVectorEncryptionCount)
	for i := 0; i < len(vectors); i++ {
		aad := []byte(fmt.Sprintf("Count-%d", i))
		encrypted := ctxI.Seal(aad, original)
		decrypted, err := ctxR.Open(aad, encrypted)
		assertNotError(t, suite, "Decryption failure", err)
		assertBytesEqual(t, suite, "Incorrect decryption", original, decrypted)

		vectors[i] = encryptionTestVector{
			plaintext:  original,
			aad:        aad,
			nonce:      ctxI.nonces[i],
			ciphertext: encrypted,
		}
	}

	return vectors, nil
}

func generateExports(t *testing.T, suite CipherSuite, ctxI *EncryptContext, ctxR *DecryptContext) ([]exporterTestVector, error) {
	vectors := make([]exporterTestVector, testVectorExportCount)
	for i := 0; i < len(vectors); i++ {
		context := []byte(fmt.Sprintf("Context-%d", i))
		exportI := ctxI.Export(context, testVectorExportLength)
		exportR := ctxR.Export(context, testVectorExportLength)
		assertBytesEqual(t, suite, "Incorrect export", exportI, exportR)
		vectors[i] = exporterTestVector{
			exportContext: context,
			exportLength:  testVectorExportLength,
			exportValue:   exportI,
		}
	}

	return vectors, nil
}

func generateTestVector(t *testing.T, setup setupMode, kemID KEMID, kdfID KDFID, aeadID AEADID) testVector {
	suite, err := AssembleCipherSuite(kemID, kdfID, aeadID)
	if err != nil {
		t.Fatalf("[%x, %x, %x] Error looking up ciphersuite: %s", kemID, kdfID, aeadID, err)
	}

	skR, pkR := mustGenerateKeyPair(t, suite)
	skE, pkE := mustGenerateKeyPair(t, suite)

	// The sender key share is only required for Auth mode variants.
	var pkS KEMPublicKey
	var skS KEMPrivateKey
	if setup.Mode == modeAuth || setup.Mode == modeAuthPSK {
		skS, pkS = mustGenerateKeyPair(t, suite)
	}

	// A PSK is only required for PSK mode variants.
	var psk []byte
	var pskID []byte
	if setup.Mode == modePSK || setup.Mode == modeAuthPSK {
		psk = fixedPSK
		pskID = fixedPSKID
	}

	suite.KEM.setEphemeralKeyPair(skE)

	enc, ctxI, err := setup.I(suite, pkR, info, skS, psk, pskID)
	assertNotError(t, suite, "Error in SetupPSKS", err)

	ctxR, err := setup.R(suite, skR, enc, info, pkS, psk, pskID)
	assertNotError(t, suite, "Error in SetupPSKR", err)

	encryptionVectors, err := generateEncryptions(t, suite, ctxI, ctxR)
	assertNotError(t, suite, "Error in generateEncryptions", err)

	exportVectors, err := generateExports(t, suite, ctxI, ctxR)
	assertNotError(t, suite, "Error in generateExports", err)

	vector := testVector{
		t:                  t,
		suite:              suite,
		mode:               setup.Mode,
		kemID:              kemID,
		kdfID:              kdfID,
		aeadID:             aeadID,
		info:               info,
		skR:                skR,
		pkR:                pkR,
		skS:                skS,
		psk:                psk,
		pskID:              pskID,
		pkS:                pkS,
		skE:                skE,
		pkE:                pkE,
		enc:                ctxI.setupParams.enc,
		zz:                 ctxI.setupParams.zz, //share secret
		keyScheduleContext: ctxI.contextParams.keyScheduleContext,
		secret:             ctxI.contextParams.secret,
		key:                ctxI.key,
		nonce:              ctxI.nonce,
		exporterSecret:     ctxI.exporterSecret,
		encryptions:        encryptionVectors,
		exports:            exportVectors,
	}

	return vector
}

func TestVectorGenerate(t *testing.T) {
	// We only generate test vectors for select ciphersuites
	supportedKEMs := []KEMID{DHKEM_X25519, DHKEM_X448, DHKEM_P256, DHKEM_P521}
	supportedKDFs := []KDFID{KDF_HKDF_SHA256, KDF_HKDF_SHA512, KDF_HKDF_SHA3_256}
	supportedAEADs := []AEADID{AEAD_AESGCM128, AEAD_AESGCM256, AEAD_CHACHA20POLY1305}

	vectors := make([]testVector, 0)
	for _, kemID := range supportedKEMs {
		for _, kdfID := range supportedKDFs {
			for _, aeadID := range supportedAEADs {
				for _, setup := range setupModes {
					vectors = append(vectors, generateTestVector(t, setup, kemID, kdfID, aeadID))
				}
			}
		}
	}

	// Encode the test vectors
	encoded, err := json.Marshal(vectors)
	if err != nil {
		t.Fatalf("Error producing test vectors: %v", err)
	}

	// Verify that we process them correctly
	verifyTestVectors(t, encoded, false)

	// Write them to a file if requested
	var outputFile string
	if outputFile = os.Getenv(outputTestVectorEnvironmentKey); len(outputFile) > 0 {
		err = ioutil.WriteFile(outputFile, encoded, 0644)
		if err != nil {
			t.Fatalf("Error writing test vectors: %v", err)
		}
	}
}

func TestVectorVerify(t *testing.T) {
	var inputFile string
	if inputFile = os.Getenv(inputTestVectorEnvironmentKey); len(inputFile) == 0 {
		t.Skip("Test vectors were not provided")
	}

	encoded, err := ioutil.ReadFile(inputFile)
	if err != nil {
		t.Fatalf("Failed reading test vectors: %v", err)
	}

	verifyTestVectors(t, encoded, true)
}

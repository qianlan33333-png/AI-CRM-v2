package migration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
)

// ArchiveAAD binds ciphertext to its run, table, source row, payload and key.
func ArchiveAAD(runID int64, sourceTable string, sourceKeyHMAC, payloadHMAC, fieldDigest []byte, keyVersion int) ([]byte, error) {
	if runID < 1 || sourceTable == "" || len(sourceKeyHMAC) != sha256.Size || len(payloadHMAC) != sha256.Size || len(fieldDigest) != sha256.Size || keyVersion < 1 {
		return nil, errors.New("invalid DM01 archive AAD")
	}
	aad := make([]byte, 10+len(sourceTable)+sha256.Size+sha256.Size+sha256.Size+4)
	binary.BigEndian.PutUint64(aad[:8], uint64(runID))
	binary.BigEndian.PutUint16(aad[8:10], uint16(len(sourceTable)))
	offset := 10
	offset += copy(aad[offset:], sourceTable)
	offset += copy(aad[offset:], sourceKeyHMAC)
	offset += copy(aad[offset:], payloadHMAC)
	offset += copy(aad[offset:], fieldDigest)
	binary.BigEndian.PutUint32(aad[offset:], uint32(keyVersion))
	return aad, nil
}

func PayloadHMAC(key, payload []byte) ([]byte, error) {
	if len(key) == 0 || len(payload) == 0 {
		return nil, errors.New("invalid DM01 payload HMAC input")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil), nil
}

// EncryptArchiveBound keeps historical 154/155 payloads out of active tables
// and logs. Its key is runtime-only and distinct from the source HMAC key.
func EncryptArchiveBound(key, aad, plaintext []byte) (nonce, ciphertext []byte, err error) {
	if len(key) != 32 || len(plaintext) == 0 {
		return nil, nil, errors.New("invalid DM01 archive encryption input")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, aad), nil
}

func DecryptArchiveBound(archiveKey, hmacKey, aad, nonce, ciphertext, wantPayloadHMAC []byte) ([]byte, error) {
	if len(archiveKey) != 32 || len(hmacKey) == 0 || len(nonce) != 12 || len(ciphertext) <= 16 || len(wantPayloadHMAC) != sha256.Size {
		return nil, errors.New("invalid DM01 archive decryption input")
	}
	block, err := aes.NewCipher(archiveKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, err
	}
	got, err := PayloadHMAC(hmacKey, plain)
	if err != nil || !hmac.Equal(got, wantPayloadHMAC) {
		return nil, errors.New("DM01 archive payload HMAC mismatch")
	}
	return plain, nil
}

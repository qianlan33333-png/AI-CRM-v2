package migration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// EncryptArchive keeps historical 154/155 payloads out of active tables and
// logs. The caller supplies a runtime-only 32-byte archive key distinct from
// the source HMAC key; only nonce/ciphertext/key version are persisted.
func EncryptArchive(key []byte, plaintext []byte) (nonce, ciphertext []byte, err error) {
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
	return nonce, gcm.Seal(nil, nonce, plaintext, nil), nil
}

package migration

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

func TestEncryptArchiveUsesDistinctNonceAndRejectsBadKey(t *testing.T) {
	key := make([]byte, 32)
	nonce, ciphertext, err := EncryptArchive(key, []byte("restricted legacy history"))
	if err != nil || len(nonce) != 12 || len(ciphertext) <= 16 {
		t.Fatalf("EncryptArchive = %x/%x/%v", nonce, ciphertext, err)
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil || string(plain) != "restricted legacy history" {
		t.Fatalf("archive decrypt = %q/%v", plain, err)
	}
	if _, _, err := EncryptArchive(make([]byte, 31), []byte("x")); err == nil {
		t.Fatal("short archive key accepted")
	}
}

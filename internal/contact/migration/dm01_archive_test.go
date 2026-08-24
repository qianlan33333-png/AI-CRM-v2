package migration

import (
	"bytes"
	"testing"
)

func TestArchiveEncryptionBindsAADAndPayloadHMAC(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	hmacKey := []byte("separate runtime hmac key")
	plain := []byte("restricted legacy history")
	payloadHMAC, err := PayloadHMAC(hmacKey, plain)
	if err != nil {
		t.Fatal(err)
	}
	aad, err := ArchiveAAD(7, "crm_user_identity_merge_audit", bytes.Repeat([]byte{2}, 32), payloadHMAC, 3)
	if err != nil {
		t.Fatal(err)
	}
	nonce, ciphertext, err := EncryptArchiveBound(key, aad, plain)
	if err != nil || len(nonce) != 12 || len(ciphertext) <= 16 {
		t.Fatalf("EncryptArchive = %x/%x/%v", nonce, ciphertext, err)
	}
	got, err := DecryptArchiveBound(key, hmacKey, aad, nonce, ciphertext, payloadHMAC)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("archive decrypt = %q/%v", got, err)
	}
	if _, err := DecryptArchiveBound(key, hmacKey, append(aad, 1), nonce, ciphertext, payloadHMAC); err == nil {
		t.Fatal("AAD tampering was accepted")
	}
	if _, _, err := EncryptArchiveBound(make([]byte, 31), aad, []byte("x")); err == nil {
		t.Fatal("short archive key accepted")
	}
}

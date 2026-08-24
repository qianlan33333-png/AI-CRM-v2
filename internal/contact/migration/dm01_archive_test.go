package migration

import (
	"bytes"
	"testing"
)

func TestArchiveEncryptionBindsAADAndPayloadHMAC(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	hmacKey := []byte("separate runtime hmac key")
	plain := []byte("restricted legacy history")
	table := "crm_user_identity_merge_audit"
	payloadHMAC, err := SourcePayloadHMAC(hmacKey, table, plain)
	if err != nil {
		t.Fatal(err)
	}
	aad, err := ArchiveAAD(7, "crm_user_identity_merge_audit", bytes.Repeat([]byte{2}, 32), payloadHMAC, bytes.Repeat([]byte{3}, 32), 3)
	if err != nil {
		t.Fatal(err)
	}
	nonce, ciphertext, err := EncryptArchiveBound(key, aad, plain)
	if err != nil || len(nonce) != 12 || len(ciphertext) <= 16 {
		t.Fatalf("EncryptArchive = %x/%x/%v", nonce, ciphertext, err)
	}
	got, err := DecryptArchiveBound(key, hmacKey, table, aad, nonce, ciphertext, payloadHMAC)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("archive decrypt = %q/%v", got, err)
	}
	if _, err := DecryptArchiveBound(key, hmacKey, table, append(aad, 1), nonce, ciphertext, payloadHMAC); err == nil {
		t.Fatal("AAD tampering was accepted")
	}
	otherFieldAAD, err := ArchiveAAD(7, "crm_user_identity_merge_audit", bytes.Repeat([]byte{2}, 32), payloadHMAC, bytes.Repeat([]byte{4}, 32), 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptArchiveBound(key, hmacKey, table, otherFieldAAD, nonce, ciphertext, payloadHMAC); err == nil {
		t.Fatal("field digest AAD drift was accepted")
	}
	if _, _, err := EncryptArchiveBound(make([]byte, 31), aad, []byte("x")); err == nil {
		t.Fatal("short archive key accepted")
	}
}

func TestDM01HMACDomainsAndTablesAreSeparated(t *testing.T) {
	key, value := bytes.Repeat([]byte{7}, 32), []byte("same")
	source, _ := SourceKeyHMAC(key, "owner_role_map", string(value))
	payload, _ := SourcePayloadHMAC(key, "owner_role_map", value)
	fields, _ := SourceFieldsHMAC(key, "owner_role_map", value)
	owner, _ := OwnerAllowlistHMAC(key, string(value))
	otherTable, _ := SourceKeyHMAC(key, "crm_user_identity", string(value))
	values := [][]byte{source, payload, fields, owner, otherTable}
	for left := range values {
		for right := left + 1; right < len(values); right++ {
			if bytes.Equal(values[left], values[right]) {
				t.Fatalf("HMAC collision between %d and %d", left, right)
			}
		}
	}
}

package archive

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"testing"
)

func TestDecryptRandomKeySupportsOfficialPKCS1Envelope(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("0123456789abcdef")
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &privateKey.PublicKey, want)
	if err != nil {
		t.Fatal(err)
	}
	provider := &SDKProvider{privateKey: privateKey}
	got, err := provider.decryptRandomKey(base64.StdEncoding.EncodeToString(ciphertext))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("decrypted key = %q", got)
	}
}

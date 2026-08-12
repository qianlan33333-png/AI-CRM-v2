package port

import (
	"context"
	"testing"
)

type keyringContractStub struct{}

func (keyringContractStub) CurrentVersion(context.Context, HMACPurpose) (HMACKeyVersion, error) {
	return 2, nil
}

func (keyringContractStub) Sign(context.Context, HMACPurpose, HMACKeyVersion, []byte) (HMACDigest, error) {
	return HMACDigest{1}, nil
}

func (keyringContractStub) Verify(context.Context, HMACPurpose, HMACKeyVersion, []byte, HMACDigest) (bool, error) {
	return true, nil
}

func TestTypedHMACKeyringContract(t *testing.T) {
	var keyring HMACKeyring = keyringContractStub{}
	version, err := keyring.CurrentVersion(context.Background(), HMACPurposeIdentityFingerprint)
	if err != nil || version != 2 {
		t.Fatalf("CurrentVersion() = %d, %v", version, err)
	}
	digest, err := keyring.Sign(context.Background(), HMACPurposeIdentityReceiptPayload, version, []byte("length-prefixed-command"))
	if err != nil || digest[0] != 1 {
		t.Fatalf("Sign() = %x, %v", digest, err)
	}
	verified, err := keyring.Verify(context.Background(), HMACPurposeIdentityReceiptPayload, 1, []byte("length-prefixed-command"), digest)
	if err != nil || !verified {
		t.Fatalf("Verify() = %v, %v", verified, err)
	}
	if HMACPurposeIdentityFingerprint == HMACPurposeIdentityReceiptPayload {
		t.Fatal("HMAC purposes must be domain separated")
	}
}

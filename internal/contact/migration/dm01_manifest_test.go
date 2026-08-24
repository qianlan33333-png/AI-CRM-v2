package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestFailsClosed(t *testing.T) {
	b := []byte(`{"contract_version":1,"source_system":"repo1","legacy_repository_sha":"2b7a80126d7becb6f95cf1ec5945dcb78a42f531","snapshot_id":"s","single_corp":true,"wecom_corp_id":"c","hmac_key_version":1,"tables":[{"name":"owner_role_map","primary_key":"userid","watermark":"updated_at+userid","schema_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mode":"full"},{"name":"crm_user_identity","primary_key":"unionid","watermark":"updated_at+unionid","schema_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mode":"full"},{"name":"wecom_external_contact_identity_map","primary_key":"id","watermark":"updated_at+id","schema_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mode":"full"}]}`)
	p := filepath.Join(t.TempDir(), "m.json")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	d := sha256.Sum256(b)
	if _, _, err := LoadManifest(p, hex.EncodeToString(d[:])); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadManifest(p, "00"); err == nil {
		t.Fatal("bad digest accepted")
	}
}

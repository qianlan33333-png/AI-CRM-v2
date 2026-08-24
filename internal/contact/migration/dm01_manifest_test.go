package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestFailsClosed(t *testing.T) {
	m := Manifest{ContractVersion: 1, SourceSystem: "repo1", LegacyRepositorySHA: LegacyRepositorySHA, SnapshotID: "s", SingleCorp: true, WeComCorpID: "c", HMACKeyVersion: 1}
	for name, spec := range map[string][2]string{
		"owner_role_map": {"userid", "updated_at+userid"}, "crm_user_identity": {"unionid", "updated_at+unionid"}, "wecom_external_contact_identity_map": {"id", "updated_at+id"},
		"crm_user_identity_merge_audit": {"id", "created_at+id"}, "crm_user_identity_resolution_queue": {"id", "updated_at+id"}, "admin_wecom_directory_members": {"id", "last_synced_at+id"},
		"contacts": {"id", "updated_at+id"}, "crm_user_identity_conflicts": {"id", "updated_at+id"}, "external_contact_bindings": {"external_userid", "updated_at+external_userid"}, "people": {"id", "updated_at+id"}, "wecom_external_contact_follow_users": {"id", "updated_at+id"},
	} {
		m.Tables = append(m.Tables, Table{Name: name, PrimaryKey: spec[0], Watermark: spec[1], SchemaDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "full", Action: manifestAction(name)})
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
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
	m.SingleCorp = false
	if err := m.Valid(); err == nil {
		t.Fatal("multi-corp owner import manifest was accepted")
	}
}

func manifestAction(name string) string {
	switch name {
	case "owner_role_map":
		return "import_staff"
	case "crm_user_identity":
		return "import_customer"
	case "wecom_external_contact_identity_map":
		return "bind_scoped_identity"
	case "crm_user_identity_merge_audit", "crm_user_identity_resolution_queue":
		return "archive_inactive"
	case "contacts":
		return "drop"
	case "admin_wecom_directory_members", "external_contact_bindings":
		return "rebuild"
	default:
		return "defer_quarantine"
	}
}

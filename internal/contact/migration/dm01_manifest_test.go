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
	m := validManifestForTest()
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

func TestManifestOwnerAllowlistIsSortedHMACOnly(t *testing.T) {
	m := validManifestForTest()
	m.OwnerAllowlistHMACs = []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if err := m.Valid(); err != nil {
		t.Fatal(err)
	}
	for name, values := range map[string][]string{
		"raw PII":   {"owner-userid"},
		"uppercase": {"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		"duplicate": {m.OwnerAllowlistHMACs[0], m.OwnerAllowlistHMACs[0]},
		"unsorted":  {m.OwnerAllowlistHMACs[1], m.OwnerAllowlistHMACs[0]},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := m
			candidate.OwnerAllowlistHMACs = values
			if err := candidate.Valid(); err == nil {
				t.Fatal("invalid owner allowlist accepted")
			}
		})
	}
}

func TestManifestIdentityScopesAreConditionalAndClosed(t *testing.T) {
	m := validManifestForTest()
	if err := m.Valid(); err != nil {
		t.Fatalf("optional scopes were required: %v", err)
	}
	m.OpenPlatformAccount = "open-platform.account-1"
	m.WeChatAppScopes = map[string]string{"wx123": "app.account-1"}
	if err := m.Valid(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Manifest){
		"unionid":      func(candidate *Manifest) { candidate.OpenPlatformAccount = " account" },
		"openid app":   func(candidate *Manifest) { candidate.WeChatAppScopes = map[string]string{"wx 123": "scope"} },
		"openid scope": func(candidate *Manifest) { candidate.WeChatAppScopes = map[string]string{"wx123": "raw scope"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := m
			mutate(&candidate)
			if err := candidate.Valid(); err == nil {
				t.Fatal("invalid identity scope accepted")
			}
		})
	}
}

func TestManifestRejectsUnsupportedReaderTable(t *testing.T) {
	m := validManifestForTest()
	m.Tables[0].Name = "unknown"
	if err := m.Valid(); err == nil {
		t.Fatal("unsupported source table was accepted")
	}
}

func validManifestForTest() Manifest {
	m := Manifest{ContractVersion: 1, SourceSystem: "repo1", LegacyRepositorySHA: LegacyRepositorySHA, SnapshotID: "s", SourceServerID: "1", SourceDatabase: "legacy", SourceReadRole: "dm01_reader", SingleCorp: true, WeComCorpID: "c", HMACKeyVersion: 1, OwnerAllowlistHMACs: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	for name, spec := range map[string][2]string{
		"owner_role_map": {"userid", "updated_at+userid"}, "crm_user_identity": {"unionid", "updated_at+unionid"}, "wecom_external_contact_identity_map": {"id", "updated_at+id"},
		"crm_user_identity_merge_audit": {"id", "created_at+id"}, "crm_user_identity_resolution_queue": {"id", "updated_at+id"}, "admin_wecom_directory_members": {"id", "last_synced_at+id"},
		"contacts": {"id", "updated_at+id"}, "crm_user_identity_conflicts": {"id", "updated_at+id"}, "external_contact_bindings": {"external_userid", "updated_at+external_userid"}, "people": {"id", "updated_at+id"}, "wecom_external_contact_follow_users": {"id", "updated_at+id"},
	} {
		m.Tables = append(m.Tables, Table{Name: name, PrimaryKey: spec[0], Watermark: spec[1], SchemaDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "full", Action: manifestAction(name)})
	}
	return m
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

package migration

import (
	"testing"
)

func TestManifestRejectsUnsupportedReaderTable(t *testing.T) {
	m := Manifest{
		ContractVersion: 1, SourceSystem: "repo1", LegacyRepositorySHA: LegacyRepositorySHA,
		SnapshotID: "snapshot", SingleCorp: true, WeComCorpID: "corp", HMACKeyVersion: 1,
		Tables: []Table{
			{Name: "owner_role_map", PrimaryKey: "userid", Watermark: "updated_at+userid", SchemaDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "full"},
			{Name: "crm_user_identity", PrimaryKey: "unionid", Watermark: "updated_at+unionid", SchemaDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "full"},
			{Name: "wecom_external_contact_identity_map", PrimaryKey: "id", Watermark: "updated_at+id", SchemaDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "full"},
			{Name: "unknown", PrimaryKey: "id", Watermark: "updated_at+id", SchemaDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "full"},
		},
	}
	if err := m.Valid(); err == nil {
		t.Fatal("unsupported source table was accepted")
	}
}

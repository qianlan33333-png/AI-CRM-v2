// Package migration contains the closed DM01 historical-import contract.
package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
)

const SourceEnvironment = "AICRM_DM01_SOURCE_DATABASE_URL"
const LegacyRepositorySHA = "2b7a80126d7becb6f95cf1ec5945dcb78a42f531"

type Manifest struct {
	ContractVersion     int               `json:"contract_version"`
	SourceSystem        string            `json:"source_system"`
	LegacyRepositorySHA string            `json:"legacy_repository_sha"`
	SnapshotID          string            `json:"snapshot_id"`
	SingleCorp          bool              `json:"single_corp"`
	WeComCorpID         string            `json:"wecom_corp_id"`
	OpenPlatformAccount string            `json:"open_platform_account,omitempty"`
	WeChatAppScopes     map[string]string `json:"wechat_app_scopes,omitempty"`
	HMACKeyVersion      int               `json:"hmac_key_version"`
	Tables              []Table           `json:"tables"`
}
type Table struct {
	Name         string `json:"name"`
	PrimaryKey   string `json:"primary_key"`
	Watermark    string `json:"watermark"`
	SchemaDigest string `json:"schema_digest"`
	Mode         string `json:"mode"`
}

func LoadManifest(path, wantHex string) (Manifest, [32]byte, error) {
	var zero [32]byte
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, zero, err
	}
	digest := sha256.Sum256(b)
	want, err := hex.DecodeString(wantHex)
	if err != nil || len(want) != 32 || !strings.EqualFold(hex.EncodeToString(digest[:]), wantHex) {
		return Manifest{}, zero, errors.New("DM01 manifest digest mismatch")
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, zero, err
	}
	if err := m.Valid(); err != nil {
		return Manifest{}, zero, err
	}
	return m, digest, nil
}
func (m Manifest) Valid() error {
	if m.ContractVersion != 1 || m.SourceSystem == "" || m.LegacyRepositorySHA != LegacyRepositorySHA || m.SnapshotID == "" || !m.SingleCorp || m.WeComCorpID == "" || m.HMACKeyVersion < 1 {
		return errors.New("invalid DM01 manifest")
	}
	want := map[string][2]string{
		"owner_role_map": {"userid", "updated_at+userid"}, "crm_user_identity": {"unionid", "updated_at+unionid"},
		"wecom_external_contact_identity_map": {"id", "updated_at+id"}, "crm_user_identity_merge_audit": {"id", "created_at+id"},
		"crm_user_identity_resolution_queue": {"id", "updated_at+id"}, "admin_wecom_directory_members": {"id", "last_synced_at+id"},
		"contacts": {"id", "updated_at+id"}, "crm_user_identity_conflicts": {"id", "updated_at+id"},
		"external_contact_bindings": {"external_userid", "updated_at+external_userid"}, "people": {"id", "updated_at+id"},
		"wecom_external_contact_follow_users": {"id", "updated_at+id"},
	}
	seen := map[string]bool{}
	for _, t := range m.Tables {
		spec, ok := want[t.Name]
		if !ok || seen[t.Name] || t.PrimaryKey != spec[0] || t.Watermark != spec[1] || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(t.SchemaDigest) || (t.Mode != "full" && t.Mode != "incremental") {
			return errors.New("invalid DM01 table manifest")
		}
		seen[t.Name] = true
	}
	for table := range want {
		if !seen[table] {
			return errors.New("missing DM01 source table")
		}
	}
	return nil
}

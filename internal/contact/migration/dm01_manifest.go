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

const LegacyRepositorySHA = "2b7a80126d7becb6f95cf1ec5945dcb78a42f531"

var (
	hexHMACPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	scopeIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	serverIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
)

type Manifest struct {
	ContractVersion     int               `json:"contract_version"`
	SourceSystem        string            `json:"source_system"`
	LegacyRepositorySHA string            `json:"legacy_repository_sha"`
	SnapshotID          string            `json:"snapshot_id"`
	SourceServerID      string            `json:"source_server_id"`
	SourceDatabase      string            `json:"source_database"`
	SourceReadRole      string            `json:"source_read_role"`
	SingleCorp          bool              `json:"single_corp"`
	WeComCorpID         string            `json:"wecom_corp_id"`
	OpenPlatformAccount string            `json:"open_platform_account,omitempty"`
	WeChatAppScopes     map[string]string `json:"wechat_app_scopes,omitempty"`
	OwnerAllowlistHMACs []string          `json:"owner_allowlist_hmacs"`
	HMACKeyVersion      int               `json:"hmac_key_version"`
	Tables              []Table           `json:"tables"`
}
type Table struct {
	Name         string `json:"name"`
	PrimaryKey   string `json:"primary_key"`
	Watermark    string `json:"watermark"`
	SchemaDigest string `json:"schema_digest"`
	Mode         string `json:"mode"`
	Action       string `json:"action"`
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
	if m.ContractVersion != 1 || m.SourceSystem == "" || m.LegacyRepositorySHA != LegacyRepositorySHA || m.SnapshotID == "" || !serverIDPattern.MatchString(m.SourceServerID) || !scopeIDPattern.MatchString(m.SourceDatabase) || !scopeIDPattern.MatchString(m.SourceReadRole) || !m.SingleCorp || m.WeComCorpID == "" || m.HMACKeyVersion < 1 || len(m.OwnerAllowlistHMACs) == 0 {
		return errors.New("invalid DM01 manifest")
	}
	for index, value := range m.OwnerAllowlistHMACs {
		if !hexHMACPattern.MatchString(value) || (index > 0 && value <= m.OwnerAllowlistHMACs[index-1]) {
			return errors.New("invalid DM01 owner allowlist")
		}
	}
	// UnionID and OpenID scopes are optional because DM01 uses unionid only as
	// a source join key and does not create an OpenID identity. When supplied
	// for a future scoped projection, both account and app scope identifiers
	// remain closed identifiers rather than arbitrary metadata.
	if m.OpenPlatformAccount != "" && !scopeIDPattern.MatchString(m.OpenPlatformAccount) {
		return errors.New("invalid DM01 unionid scope")
	}
	for appID, scope := range m.WeChatAppScopes {
		if !scopeIDPattern.MatchString(appID) || !scopeIDPattern.MatchString(scope) {
			return errors.New("invalid DM01 openid scope")
		}
	}
	want := map[string][3]string{
		"owner_role_map": {"userid", "updated_at+userid", "import_staff"}, "crm_user_identity": {"unionid", "updated_at+unionid", "import_customer"},
		"wecom_external_contact_identity_map": {"id", "updated_at+id", "bind_scoped_identity"}, "crm_user_identity_merge_audit": {"id", "created_at+id", "archive_inactive"},
		"crm_user_identity_resolution_queue": {"id", "updated_at+id", "archive_inactive"}, "admin_wecom_directory_members": {"id", "last_synced_at+id", "rebuild"},
		"contacts": {"id", "updated_at+id", "drop"}, "crm_user_identity_conflicts": {"id", "updated_at+id", "defer_quarantine"},
		"external_contact_bindings": {"external_userid", "updated_at+external_userid", "rebuild"}, "people": {"id", "updated_at+id", "defer_quarantine"},
		"wecom_external_contact_follow_users": {"id", "updated_at+id", "defer_quarantine"},
	}
	seen := map[string]bool{}
	for _, t := range m.Tables {
		spec, ok := want[t.Name]
		if !ok || seen[t.Name] || t.PrimaryKey != spec[0] || t.Watermark != spec[1] || t.Action != spec[2] || !hexHMACPattern.MatchString(t.SchemaDigest) || (t.Mode != "full" && t.Mode != "incremental") {
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

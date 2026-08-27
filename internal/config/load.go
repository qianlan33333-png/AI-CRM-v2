package config

import (
	"os"
	"strings"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

// Load reads startup environment exactly once through the typed config boundary.
func Load(role appruntime.Role) (Root, error) {
	return load(role, os.LookupEnv)
}

type DM01Runtime struct {
	SourceDatabaseURL string
	TargetDatabaseURL string
	SourceHMACKey     string
	ArchiveKey        string
	SourceAllowlist   []string
}

type V1ArchiveRuntime struct {
	SourceDatabaseURL string
	TargetDatabaseURL string
	SourceHMACKey     string
	ArchiveKey        string
}

func LoadDM01RuntimeEnvironment() DM01Runtime {
	return DM01Runtime{
		SourceDatabaseURL: os.Getenv("AICRM_DM01_SOURCE_DATABASE_URL"),
		TargetDatabaseURL: os.Getenv("AICRM_DATABASE_URL"),
		SourceHMACKey:     os.Getenv("AICRM_DM01_SOURCE_HMAC_KEY"),
		ArchiveKey:        os.Getenv("AICRM_DM01_ARCHIVE_KEY"),
		SourceAllowlist:   splitDM01SourceAllowlist(os.Getenv("AICRM_DM01_SOURCE_IDENTITY_ALLOWLIST")),
	}
}

func LoadV1ArchiveRuntimeEnvironment() V1ArchiveRuntime {
	return V1ArchiveRuntime{
		SourceDatabaseURL: os.Getenv("AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL"),
		TargetDatabaseURL: os.Getenv("AICRM_V1_ARCHIVE_TARGET_DATABASE_URL"),
		SourceHMACKey:     os.Getenv("AICRM_V1_ARCHIVE_SOURCE_HMAC_KEY"),
		ArchiveKey:        os.Getenv("AICRM_V1_ARCHIVE_ENCRYPTION_KEY"),
	}
}

func splitDM01SourceAllowlist(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

package config

import (
	"os"

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
}

func LoadDM01RuntimeEnvironment() DM01Runtime {
	return DM01Runtime{
		SourceDatabaseURL: os.Getenv("AICRM_DM01_SOURCE_DATABASE_URL"),
		TargetDatabaseURL: os.Getenv("AICRM_DATABASE_URL"),
		SourceHMACKey:     os.Getenv("AICRM_DM01_SOURCE_HMAC_KEY"),
		ArchiveKey:        os.Getenv("AICRM_DM01_ARCHIVE_KEY"),
	}
}

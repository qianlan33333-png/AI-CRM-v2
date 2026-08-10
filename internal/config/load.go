package config

import (
	"os"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

// Load reads startup environment exactly once through the typed config boundary.
func Load(role appruntime.Role) (Root, error) {
	return load(role, os.LookupEnv)
}

package config

import (
	"errors"
	"testing"

	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

func TestValidateSettingRegistry(t *testing.T) {
	tests := []struct {
		name    string
		key     configport.Key
		value   string
		want    string
		wantErr error
	}{
		{name: "corp", key: configport.WeComCorpID, value: `"corp-1"`, want: `"corp-1"`},
		{name: "agent", key: configport.WeComAgentID, value: `7`, want: `7`},
		{name: "rate upper", key: configport.OutboundRatePerSecond, value: `50`, want: `50`},
		{name: "rate invalid", key: configport.OutboundRatePerSecond, value: `51`, wantErr: configport.ErrInvalidSetting},
		{name: "fraction rejected", key: configport.OutboundMaxAttempts, value: `1.0`, wantErr: configport.ErrInvalidSetting},
		{name: "unknown", key: "custom.key", value: `1`, wantErr: configport.ErrUnknownSetting},
		{name: "secret", key: configport.WeComSecret, value: `"sentinel"`, wantErr: configport.ErrSecretSetting},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateSetting(test.key, []byte(test.value))
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ValidateSetting() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil || string(got) != test.want {
				t.Fatalf("ValidateSetting() = %s, %v; want %s, nil", got, err, test.want)
			}
		})
	}
}

func TestSecretKeysAreNeverReadable(t *testing.T) {
	for _, key := range []configport.Key{
		configport.DatabaseURL, configport.WeComSecret, configport.WeComCallbackToken,
		configport.WeComCallbackAESKey, configport.AIAPIKey, configport.AuthJWTSecret,
		configport.ExtensionAPIKeyPepper, configport.WebhookMasterKey,
	} {
		if err := ValidateReadableSetting(key); !errors.Is(err, configport.ErrSecretSetting) {
			t.Fatalf("ValidateReadableSetting(%s) error = %v, want ErrSecretSetting", key, err)
		}
	}
}

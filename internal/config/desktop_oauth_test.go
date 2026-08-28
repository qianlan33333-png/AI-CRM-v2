package config

import (
	"fmt"
	"strings"
	"testing"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

func TestLoadWeComOAuthDesktopAgentID(t *testing.T) {
	const secret = "desktop-oauth-secret-sentinel"
	base := map[string]string{
		databaseURLEnv:        "postgres://db/aicrm",
		apiListenAddressEnv:   "127.0.0.1:8080",
		apiPoolMaxConnsEnv:    "1",
		identityHMACKeyEnv:    strings.Repeat("A", 43),
		weComOAuthCorpIDEnv:   "corp-fixture",
		weComOAuthSecretEnv:   secret,
		weComOAuthCallbackEnv: "https://crm.example.test/auth/wecom/callback",
	}

	for _, test := range []struct {
		name        string
		agentID     string
		present     bool
		wantAgent   int64
		wantProblem string
	}{
		{name: "missing", wantAgent: 0},
		{name: "empty", agentID: "", present: true, wantProblem: "wecom.oauth.agent_id must be a positive integer"},
		{name: "zero", agentID: "0", present: true, wantProblem: "wecom.oauth.agent_id must be a positive integer"},
		{name: "negative", agentID: "-1", present: true, wantProblem: "wecom.oauth.agent_id must be a positive integer"},
		{name: "leading zero", agentID: "01", present: true, wantProblem: "wecom.oauth.agent_id must be a positive integer"},
		{name: "overflow", agentID: "9223372036854775808", present: true, wantProblem: "wecom.oauth.agent_id must be a positive integer"},
		{name: "valid", agentID: "42", present: true, wantAgent: 42},
	} {
		t.Run(test.name, func(t *testing.T) {
			values := cloneValues(base)
			if test.present {
				values[weComOAuthAgentIDEnv] = test.agentID
			}
			root, err := load(appruntime.RoleAPI, mapLookup(values))
			if test.wantProblem != "" {
				if err == nil || err.Error() != "invalid startup configuration: "+test.wantProblem {
					t.Fatalf("load() error = %v, want %q", err, test.wantProblem)
				}
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("load() error leaked secret: %q", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !root.WeCom.OAuth.Enabled || root.WeCom.OAuth.AgentID != test.wantAgent {
				t.Fatalf("OAuth = %#v, want enabled AgentID=%d", root.WeCom.OAuth, test.wantAgent)
			}
			for _, formatted := range []string{fmt.Sprint(root), fmt.Sprintf("%#v", root), fmt.Sprintf("%+v", root)} {
				if strings.Contains(formatted, secret) {
					t.Fatalf("Root formatting leaked secret: %q", formatted)
				}
			}
		})
	}
}

func TestLoadWeComOAuthDesktopAgentIDRequiresOAuthTriple(t *testing.T) {
	values := map[string]string{
		databaseURLEnv:       "postgres://db/aicrm",
		apiListenAddressEnv:  "127.0.0.1:8080",
		apiPoolMaxConnsEnv:   "1",
		identityHMACKeyEnv:   strings.Repeat("A", 43),
		weComOAuthAgentIDEnv: "42",
	}
	_, err := load(appruntime.RoleAPI, mapLookup(values))
	const want = "invalid startup configuration: wecom.oauth requires corp_id, secret, and callback_url together"
	if err == nil || err.Error() != want {
		t.Fatalf("load() error = %v, want %q", err, want)
	}
}

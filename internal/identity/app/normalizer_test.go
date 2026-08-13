package app

import (
	"errors"
	"testing"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestNormalizeFrozenKindsScopesAndValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		ref   identityport.IDRef
		value string
	}{
		{"wecom external user ID", identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp-a", Value: "  wx-user  "}, "wx-user"},
		{"union ID", identityport.IDRef{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:account-a", Value: " union-1 "}, "union-1"},
		{"mini-program open ID", identityport.IDRef{Kind: identityport.KindMPOpenID, Scope: "wechat-app:wx-mini", Value: "open-mini"}, "open-mini"},
		{"OA open ID", identityport.IDRef{Kind: identityport.KindOAOpenID, Scope: "wechat-app:wx-oa", Value: "open-oa"}, "open-oa"},
		{"Alipay user ID", identityport.IDRef{Kind: identityport.KindAlipayUserID, Scope: "alipay-app:2026", Value: "ali-user"}, "ali-user"},
		{"phone E.164", identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 "}, "+8613800138000"},
		{"extension", identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:partner-a", Value: "record-1"}, "record-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Normalize(test.ref)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != test.ref.Kind || got.Scope != test.ref.Scope || got.NormalizedValue != test.value || got.NormalizerVersion != 1 {
				t.Fatalf("normalized=%+v", got)
			}
		})
	}
}

func TestNormalizeRejectsUnknownNamespacesAndInvalidPhone(t *testing.T) {
	for _, ref := range []identityport.IDRef{
		{Kind: identityport.KindUnionID, Scope: "wecom-corp:corp-a", Value: "union"},
		{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "13800138000"},
		{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+00 13800138000"},
		{Kind: identityport.KindExtension, Scope: "ext:", Value: "id"},
		{Kind: identityport.IDKind("unknown"), Scope: "ext:partner", Value: "id"},
		{Kind: identityport.KindMPOpenID, Scope: "wechat-app: app", Value: "id"},
		{Kind: identityport.KindMPOpenID, Scope: "wechat-app:app id", Value: "id"},
	} {
		if _, err := Normalize(ref); !errors.Is(err, ErrInvalidIdentity) {
			t.Fatalf("Normalize(%+v) error=%v, want invalid identity", ref, err)
		}
	}
}

func TestValidateNormalizedRejectsNonCanonicalValues(t *testing.T) {
	valid, err := Normalize(identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+8613800138000"})
	if err != nil || ValidateNormalized(valid) != nil {
		t.Fatalf("valid normalized identity=%+v err=%v", valid, err)
	}
	valid.NormalizedValue = "+86 13800138000"
	if !errors.Is(ValidateNormalized(valid), ErrInvalidIdentity) {
		t.Fatal("non-canonical normalized phone was accepted")
	}
}

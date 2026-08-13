// Package app implements Identity-owned business behavior.
package app

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

const NormalizerVersion int16 = 1

var (
	ErrInvalidIdentity = errors.New("invalid identity")
	phoneE164          = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)
)

// NormalizedIdentity contains the only identity value that may cross into
// Identity-owned persistence. Raw caller input is never retained.
type NormalizedIdentity struct {
	Kind              identityport.IDKind
	Scope             string
	NormalizedValue   string
	NormalizerVersion int16
}

// Normalize validates the frozen kind/scope namespace and deterministically
// maps an input value to its v1 storage representation.
func Normalize(ref identityport.IDRef) (NormalizedIdentity, error) {
	scope := strings.TrimSpace(ref.Scope)
	value := strings.TrimSpace(ref.Value)
	if !validScope(ref.Kind, scope) || value == "" || len(value) > 1024 || containsControl(value) {
		return NormalizedIdentity{}, ErrInvalidIdentity
	}

	if ref.Kind == identityport.KindPhone {
		value = compactPhone(value)
		if !phoneE164.MatchString(value) {
			return NormalizedIdentity{}, ErrInvalidIdentity
		}
	}

	return NormalizedIdentity{
		Kind:              ref.Kind,
		Scope:             scope,
		NormalizedValue:   value,
		NormalizerVersion: NormalizerVersion,
	}, nil
}

// ValidateNormalized prevents an internal caller from bypassing the frozen
// normalizer before passing a value to the transaction-bound store.
func ValidateNormalized(identity NormalizedIdentity) error {
	canonical, err := Normalize(identityport.IDRef{
		Kind:  identity.Kind,
		Scope: identity.Scope,
		Value: identity.NormalizedValue,
	})
	if err != nil || canonical != identity {
		return ErrInvalidIdentity
	}
	return nil
}

func validScope(kind identityport.IDKind, scope string) bool {
	if len(scope) > 256 || strings.IndexFunc(scope, unicode.IsSpace) >= 0 || containsControl(scope) {
		return false
	}
	switch kind {
	case identityport.KindWeComExternalUserID:
		return hasScopeNamespace(scope, "wecom-corp:")
	case identityport.KindUnionID:
		return hasScopeNamespace(scope, "wechat-open-platform:")
	case identityport.KindMPOpenID, identityport.KindOAOpenID:
		return hasScopeNamespace(scope, "wechat-app:")
	case identityport.KindAlipayUserID:
		return hasScopeNamespace(scope, "alipay-app:")
	case identityport.KindPhone:
		return scope == "phone:e164"
	case identityport.KindExtension:
		return hasScopeNamespace(scope, "ext:")
	default:
		return false
	}
}

func hasScopeNamespace(scope, prefix string) bool {
	return strings.HasPrefix(scope, prefix) && len(scope) > len(prefix)
}

func compactPhone(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '+' || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return ""
		}
	}
	return builder.String()
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

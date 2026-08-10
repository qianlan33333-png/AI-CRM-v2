package p1s11_test

import (
	"reflect"
	"strings"
	"testing"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	runtimegenerated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

func TestGeneratedCustomerIsChannelNeutral(t *testing.T) {
	typeOf := reflect.TypeOf(generated.Customer{})
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		for _, forbidden := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
			if name == forbidden {
				t.Fatalf("generated Customer contains %s", forbidden)
			}
		}
	}
}

func TestGeneratedIdentityRefRequiresScopedEvidence(t *testing.T) {
	typeOf := reflect.TypeOf(generated.IdentityRef{})
	want := map[string]bool{"type": true, "scope": true, "value": true, "assurance": true, "source": true}
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		delete(want, name)
		if field.Type.Kind() == reflect.Pointer {
			t.Fatalf("IdentityRef.%s became optional", field.Name)
		}
	}
	if len(want) > 0 {
		t.Fatalf("IdentityRef missing required fields: %v", want)
	}
}

func TestPublicPortSurfaceIsFrozen(t *testing.T) {
	contracts := map[string]struct {
		value   any
		methods int
	}{
		"contact.MergePort":   {(*contactport.MergePort)(nil), 3},
		"config.Service":      {(*configport.Service)(nil), 2},
		"events.Appender":     {(*eventport.Appender)(nil), 1},
		"identity.Service":    {(*identityport.Service)(nil), 3},
		"auth.Service":        {(*authport.Service)(nil), 3},
		"platform.UnitOfWork": {(*platformport.UnitOfWork)(nil), 1},
	}
	for name, contract := range contracts {
		if got := reflect.TypeOf(contract.value).Elem().NumMethod(); got != contract.methods {
			t.Errorf("%s methods=%d want=%d", name, got, contract.methods)
		}
	}

	for _, command := range []any{contactport.CreateForIdentityCommand{}, contactport.MergeCustomersCommand{}, contactport.ExternalEventCommand{}} {
		typeOf := reflect.TypeOf(command)
		for index := 0; index < typeOf.NumField(); index++ {
			lower := strings.ToLower(typeOf.Field(index).Name)
			if strings.Contains(lower, "external") || strings.Contains(lower, "union") || strings.Contains(lower, "openid") || strings.Contains(lower, "phone") {
				t.Fatalf("contact command leaks external identity: %s.%s", typeOf.Name(), typeOf.Field(index).Name)
			}
		}
	}
}

func TestCandidateServerIsNotTheRuntimeServer(t *testing.T) {
	if got := reflect.TypeOf((*runtimegenerated.StrictServerInterface)(nil)).Elem().NumMethod(); got != 1 {
		t.Fatalf("runtime server methods=%d want health-only 1", got)
	}
	if got := reflect.TypeOf((*generated.StrictServerInterface)(nil)).Elem().NumMethod(); got != 10 {
		t.Fatalf("candidate server methods=%d want pending 10", got)
	}
}

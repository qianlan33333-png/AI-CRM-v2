package p3c00_test

import (
	"reflect"
	"strings"
	"testing"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestCustomerListUsesFrozenKeysetFilters(t *testing.T) {
	typeOf := reflect.TypeOf(generated.ListCustomersParams{})
	want := map[string]bool{
		"cursor": true, "limit": true, "keyword": true, "owner_staff_id": true,
		"stage_id": true, "channel_id": true, "tag_id": true, "is_deleted": true,
		"added_after": true, "added_before": true, "last_interact_after": true,
		"last_interact_before": true,
	}
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("form"), ",")[0]
		if name == "offset" {
			t.Fatal("offset pagination was reintroduced")
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing contact filters: %v", want)
	}
}

func TestContactPortRemainsChannelNeutralAndExact(t *testing.T) {
	portType := reflect.TypeOf((*contactport.MergePort)(nil)).Elem()
	wantMethods := []string{"AppendExternalEvent", "CreateForIdentity", "MergeCustomers"}
	if portType.NumMethod() != len(wantMethods) {
		t.Fatalf("MergePort methods=%d want=%d", portType.NumMethod(), len(wantMethods))
	}
	for index, method := range wantMethods {
		if got := portType.Method(index).Name; got != method {
			t.Fatalf("MergePort method[%d]=%q want=%q", index, got, method)
		}
	}
	for _, value := range []any{
		generated.Customer{}, contactport.CreateForIdentityCommand{},
		contactport.MergeCustomersCommand{}, contactport.ExternalEventCommand{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			name := strings.ToLower(typeOf.Field(index).Name + " " + typeOf.Field(index).Tag.Get("json"))
			for _, forbidden := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s leaks %s", typeOf.Name(), forbidden)
				}
			}
		}
	}
}

func TestContactResponsesFreezeCountEstimateAndActor(t *testing.T) {
	listType := reflect.TypeOf(generated.CustomerListResponse{})
	for _, field := range []string{"Items", "NextCursor", "Total", "TotalIsEstimate", "Watermark"} {
		if _, ok := listType.FieldByName(field); !ok {
			t.Fatalf("CustomerListResponse missing %s", field)
		}
	}
	eventType := reflect.TypeOf(generated.CustomerEvent{})
	actor, ok := eventType.FieldByName("Actor")
	if !ok || actor.Type.Kind() == reflect.Pointer {
		t.Fatal("CustomerEvent.Actor must remain required")
	}
}

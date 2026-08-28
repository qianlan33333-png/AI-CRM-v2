package app

import (
	"reflect"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// This locks the store/reconcile contract to every typed Port field. A changed
// private fact may be rejected by validation, but it must never retain digest.
func TestDeferredIdentityHistoryDigestBindsEveryPortField(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	person := deferredPerson(at)
	person.ID, person.RedactedRoots = 1, []string{"mobile", "third_party_user_id"}
	conflict := deferredConflict(at)
	conflict.ID, conflict.RedactedRoots = 1, []string{"payload", "resolution"}
	missing := missingRoot(at)
	missing.ID, missing.RedactedRoots = 1, []string{"raw_profile", "avatar"}

	cases := []struct {
		name   string
		value  any
		digest func(any) ([32]byte, error)
	}{
		{"person", person, func(value any) ([32]byte, error) {
			return HistoricalDeferredPersonDigest(value.(contact.HistoricalDeferredPerson))
		}},
		{"conflict", conflict, func(value any) ([32]byte, error) {
			return HistoricalDeferredIdentityConflictDigest(value.(contact.HistoricalDeferredIdentityConflict))
		}},
		{"missing_root", missing, func(value any) ([32]byte, error) {
			return HistoricalMissingRootIdentityDigest(value.(contact.HistoricalMissingRootIdentity))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before, err := test.digest(test.value)
			if err != nil {
				t.Fatal(err)
			}
			base := reflect.ValueOf(test.value)
			for field := range base.NumField() {
				field := field
				t.Run(base.Type().Field(field).Name, func(t *testing.T) {
					candidate := reflect.New(base.Type()).Elem()
					candidate.Set(base)
					if !mutateDeferredDigestField(candidate.Field(field), at) {
						t.Fatalf("unsupported field type %s", candidate.Field(field).Type())
					}
					after, err := test.digest(candidate.Interface())
					if err == nil && after == before {
						t.Fatalf("digest omitted %s", base.Type().Field(field).Name)
					}
				})
			}
		})
	}
}

func TestDeferredIdentityHistoryDigestBindsOptionalAndRootOrder(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	conflict := deferredConflict(at)
	conflict.ID, conflict.RedactedRoots = 1, []string{"payload", "resolution"}
	assertDeferredDigestDiff(t, HistoricalDeferredIdentityConflictDigest, conflict, func(value *contact.HistoricalDeferredIdentityConflict) { value.ResolvedAt = nil })
	assertDeferredDigestDiff(t, HistoricalDeferredIdentityConflictDigest, conflict, func(value *contact.HistoricalDeferredIdentityConflict) {
		value.RedactedRoots[0], value.RedactedRoots[1] = value.RedactedRoots[1], value.RedactedRoots[0]
	})

	missing := missingRoot(at)
	missing.ID, missing.RedactedRoots = 1, []string{"raw_profile", "avatar"}
	assertDeferredDigestDiff(t, HistoricalMissingRootIdentityDigest, missing, func(value *contact.HistoricalMissingRootIdentity) { kind := int32(-7); value.Type = &kind })
	assertDeferredDigestDiff(t, HistoricalMissingRootIdentityDigest, missing, func(value *contact.HistoricalMissingRootIdentity) {
		gender := deferredDigest(99)
		value.GenderDigest = &gender
	})
	assertDeferredDigestDiff(t, HistoricalMissingRootIdentityDigest, missing, func(value *contact.HistoricalMissingRootIdentity) {
		value.RedactedRoots[0], value.RedactedRoots[1] = value.RedactedRoots[1], value.RedactedRoots[0]
	})

	person := deferredPerson(at)
	person.ID, person.RedactedRoots = 1, []string{"mobile", "third_party_user_id"}
	assertDeferredDigestDiff(t, HistoricalDeferredPersonDigest, person, func(value *contact.HistoricalDeferredPerson) {
		value.RedactedRoots[0], value.RedactedRoots[1] = value.RedactedRoots[1], value.RedactedRoots[0]
	})
}

func assertDeferredDigestDiff[T any](t *testing.T, digest func(T) ([32]byte, error), value T, mutate func(*T)) {
	t.Helper()
	before, err := digest(value)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&value)
	after, err := digest(value)
	if err == nil && after == before {
		t.Fatal("digest omitted optional or root ordering fact")
	}
}

func mutateDeferredDigestField(field reflect.Value, at time.Time) bool {
	switch field.Kind() {
	case reflect.Int64:
		field.SetInt(field.Int() + 1)
	case reflect.String:
		field.SetString(field.String() + "~")
	case reflect.Array:
		if field.Type().Elem().Kind() != reflect.Uint8 || field.Len() != 32 {
			return false
		}
		field.Index(0).SetUint(field.Index(0).Uint() ^ 1)
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return false
		}
		values := append([]string(nil), field.Interface().([]string)...)
		if len(values) == 0 {
			values = []string{"root"}
		} else {
			values[0] += "~"
		}
		field.Set(reflect.ValueOf(values))
	case reflect.Struct:
		if field.Type() != reflect.TypeOf(time.Time{}) {
			return false
		}
		field.Set(reflect.ValueOf(field.Interface().(time.Time).Add(time.Microsecond)))
	case reflect.Ptr:
		value := reflect.New(field.Type().Elem())
		if field.IsNil() {
			switch value.Elem().Kind() {
			case reflect.Int32:
				value.Elem().SetInt(-1)
			case reflect.Array:
				value.Elem().Index(0).SetUint(1)
			case reflect.Struct:
				if value.Elem().Type() != reflect.TypeOf(time.Time{}) {
					return false
				}
				value.Elem().Set(reflect.ValueOf(at))
			default:
				return false
			}
		} else {
			value.Elem().Set(field.Elem())
			switch value.Elem().Kind() {
			case reflect.Int32:
				value.Elem().SetInt(value.Elem().Int() + 1)
			case reflect.Array:
				value.Elem().Index(0).SetUint(value.Elem().Index(0).Uint() ^ 1)
			case reflect.Struct:
				if value.Elem().Type() != reflect.TypeOf(time.Time{}) {
					return false
				}
				value.Elem().Set(reflect.ValueOf(value.Elem().Interface().(time.Time).Add(time.Microsecond)))
			default:
				return false
			}
		}
		field.Set(value)
	default:
		return false
	}
	return true
}

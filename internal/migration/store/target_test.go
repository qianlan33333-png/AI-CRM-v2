package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
)

func TestTargetRegistryRequiresPairedClosedOperations(t *testing.T) {
	apply := func(context.Context, pgx.Tx, migration.LeaseFence, []byte) error { return nil }
	verify := func(context.Context, pgx.Tx, migration.ResultReceipt) error { return nil }
	registry, err := NewTargetRegistry(
		map[string]ApplyOperation{"fixture": apply},
		map[string]VerifyOperation{"fixture": verify},
	)
	if err != nil || registry == nil || registry.apply["fixture"] == nil || registry.verify["fixture"] == nil {
		t.Fatalf("registry=%#v err=%v", registry, err)
	}
	for _, test := range []struct {
		name   string
		apply  map[string]ApplyOperation
		verify map[string]VerifyOperation
	}{
		{name: "empty", apply: nil, verify: nil},
		{name: "unverified", apply: map[string]ApplyOperation{"fixture": apply}, verify: nil},
		{name: "verify-only", apply: nil, verify: map[string]VerifyOperation{"fixture": verify}},
		{name: "empty-name", apply: map[string]ApplyOperation{"": apply}, verify: map[string]VerifyOperation{"": verify}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewTargetRegistry(test.apply, test.verify); !errors.Is(err, migration.ErrInvalidManifest) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

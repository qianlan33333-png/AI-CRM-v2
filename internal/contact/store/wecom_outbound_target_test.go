package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

type weComOutboundTargetFake struct {
	params contactdb.ResolveVerifiedWeComOutboundTargetParams
	row    contactdb.ResolveVerifiedWeComOutboundTargetRow
	err    error
}

func (fake *weComOutboundTargetFake) ResolveVerifiedWeComOutboundTarget(_ context.Context, params contactdb.ResolveVerifiedWeComOutboundTargetParams) (contactdb.ResolveVerifiedWeComOutboundTargetRow, error) {
	fake.params = params
	return fake.row, fake.err
}

func TestWeComOutboundTargetResolverReturnsExactScopedFacts(t *testing.T) {
	fake := &weComOutboundTargetFake{row: contactdb.ResolveVerifiedWeComOutboundTargetRow{SenderWecomUserid: "owner-1", ExternalUserid: "external-1"}}
	resolver, err := newWeComOutboundTargetResolver(fake, "corp-1")
	if err != nil {
		t.Fatal(err)
	}
	sender, externalUserID, resolved, err := resolver.Resolve(context.Background(), 41)
	if err != nil || !resolved || sender != "owner-1" || externalUserID != "external-1" || fake.params.CustomerID != 41 || fake.params.CorpID != "corp-1" {
		t.Fatalf("sender=%q external=%q resolved=%t params=%+v err=%v", sender, externalUserID, resolved, fake.params, err)
	}
}

func TestWeComOutboundTargetResolverSeparatesUnresolvedFromInfrastructureFailure(t *testing.T) {
	for name, test := range map[string]struct {
		err          error
		wantResolved bool
		wantErr      bool
	}{
		"no exact target": {err: pgx.ErrNoRows},
		"database error":  {err: errors.New("database unavailable"), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			resolver, err := newWeComOutboundTargetResolver(&weComOutboundTargetFake{err: test.err}, "corp-1")
			if err != nil {
				t.Fatal(err)
			}
			_, _, resolved, err := resolver.Resolve(context.Background(), 41)
			if resolved != test.wantResolved || (err != nil) != test.wantErr {
				t.Fatalf("resolved=%t err=%v", resolved, err)
			}
		})
	}
}

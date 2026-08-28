package store

import (
	"bytes"
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
)

func TestHistoricalReferenceEvidencePreservesAssurance(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	row := identitydb.LockHistoricalScopedWeComIdentityEvidenceRow{ID: 9, CustomerID: pgtype.Int8{Int64: 11, Valid: true}, Kind: "wecom_external_userid", Scope: "wecom-corp:corp", NormalizedValue: "external", FingerprintKeyVersion: 2, ReviewFingerprint: key[:16]}
	for _, assurance := range []string{"declared", "verified"} {
		row.Assurance = assurance
		got, err := historicalReferenceEvidence(row, key)
		if err != nil || got.IdentityID != row.ID || got.Assurance != identityport.Assurance(assurance) || got.HMACKeyVersion != 2 || got.Scope != row.Scope || got.ExternalUserID != row.NormalizedValue {
			t.Fatal("evidence did not preserve identity facts", err)
		}
	}
	for name, mutate := range map[string]func(*identitydb.LockHistoricalScopedWeComIdentityEvidenceRow){
		"missing customer":   func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) { v.CustomerID.Valid = false },
		"wrong kind":         func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) { v.Kind = "unionid" },
		"unknown assurance":  func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) { v.Assurance = "unknown" },
		"bad scope":          func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) { v.Scope = "wecom-corp: bad" },
		"noncanonical value": func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) { v.NormalizedValue = " external " },
		"wrong fingerprint": func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) {
			v.ReviewFingerprint = bytes.Repeat([]byte{8}, 16)
		},
		"missing key version": func(v *identitydb.LockHistoricalScopedWeComIdentityEvidenceRow) { v.FingerprintKeyVersion = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := row
			mutate(&bad)
			if _, err := historicalReferenceEvidence(bad, key); err == nil {
				t.Fatal("accepted invalid historical reference")
			}
		})
	}
	if _, err := historicalReferenceEvidence(row, key[:16]); err == nil {
		t.Fatal("accepted short key")
	}
	if _, err := NewRepository().LockHistoricalScopedWeComIdentityEvidence(context.Background(), 9, key); err == nil {
		t.Fatal("evidence read must require caller transaction")
	}
}

package store

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/profile"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

func TestProfileEffectRowPreservesAttemptAndReconcileFacts(t *testing.T) {
	at := time.Date(2026, time.August, 26, 8, 0, 0, 0, time.UTC)
	got := fromProfileRow(wecomdb.WecomContactProfileEffect{
		EffectID: 7, LegacyReceiptID: 8, ActorID: 9, CorpID: "corp", StaffUserid: "staff", ExternalUserid: "external",
		Remark: "remark", Description: "description", IdempotencyDigest: "sha256:idem", EnvelopeFingerprint: "sha256:envelope",
		State: string(eer.StateReconciled), AcceptReceiptID: 10,
		QueueReceiptID: pgtype.Int8{Int64: 11, Valid: true}, RiverJobID: pgtype.Int8{Int64: 12, Valid: true},
		Generation: 13, Fence: 14, LeaseExpiresAt: pgtype.Timestamptz{Time: at, Valid: true},
		AttemptReceiptID: pgtype.Int8{Int64: 15, Valid: true}, AttemptReceiptDigest: pgtype.Text{String: "sha256:attempt", Valid: true},
		AttemptCompletedAt: pgtype.Timestamptz{Time: at, Valid: true}, ProviderCallAttempted: true, RealExternalCallExecuted: true,
		ReconcileReceiptID: pgtype.Int8{Int64: 16, Valid: true}, ReconcileReceiptDigest: pgtype.Text{String: "sha256:reconcile", Valid: true},
		ReconcileEvidenceDigest: pgtype.Text{String: "sha256:evidence", Valid: true}, ReconcileResolution: pgtype.Text{String: string(profile.ResolutionProviderApplied), Valid: true},
		ReconciledAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true},
	})

	if got.EffectID != "eer_7" || got.QueueReceiptID != "eerop_11" || got.RiverJobID != 12 ||
		got.AttemptReceiptID != "eerop_15" || got.AttemptReceiptDigest != "sha256:attempt" ||
		!got.ProviderCallAttempted || !got.RealExternalCallExecuted || got.ReconcileReceiptID != "eerop_16" ||
		got.ReconcileResolution != profile.ResolutionProviderApplied || !got.LeaseExpiresAt.Equal(at) ||
		!got.AttemptCompletedAt.Equal(at) || !got.ReconciledAt.Equal(at) || !got.UpdatedAt.Equal(at) {
		t.Fatalf("profile effect=%+v", got)
	}
}

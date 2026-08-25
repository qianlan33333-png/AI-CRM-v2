package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

var (
	ErrInboundStore     = errors.New("WeCom inbound store failure")
	ErrInboundClaimLost = errors.New("WeCom inbound claim was lost")
)

type InboundRepository struct{}

var _ wecomapp.InboundStore = (*InboundRepository)(nil)
var _ wecomapp.SyncHandoffStore = (*InboundRepository)(nil)

func NewInboundRepository() *InboundRepository { return &InboundRepository{} }

func (repository *InboundRepository) ReserveInbound(ctx context.Context, envelope wecomapp.InboundEnvelope) (wecomapp.InboundReservation, error) {
	if repository == nil || !validEnvelope(envelope) {
		return wecomapp.InboundReservation{}, wecomapp.ErrInvalidInboundMessage
	}
	queries, err := inboundQueries(ctx)
	if err != nil {
		return wecomapp.InboundReservation{}, err
	}
	digest := sha256.Sum256(envelope.RawPayload)
	row, err := queries.InsertWeComContactInbox(ctx, wecomdb.InsertWeComContactInboxParams{
		Source: string(envelope.Source), SourceKey: envelope.SourceKey, CorpID: envelope.CorpID,
		EventType: envelope.EventType, ExternalUserid: envelope.ExternalUserID,
		RawPayload: envelope.RawPayload, PayloadDigest: "sha256:" + hex.EncodeToString(digest[:]),
		OccurredAt: pgtype.Timestamptz{Time: envelope.OccurredAt.UTC(), Valid: true}, State: initialState(envelope),
		ExternalContactChangeType: externalContactChangeType(envelope), ExternalContactWecomUserid: externalContactUserID(envelope),
		ExternalContactState: externalContactState(envelope), ExternalContactWelcomePresent: externalContactWelcomePresent(envelope),
		ExternalContactWelcomeDigest: externalContactWelcomeDigest(envelope), ExternalContactSourceDigest: externalContactSourceDigest(envelope),
		ExternalContactFailReasonDigest: externalContactFailReasonDigest(envelope),
	})
	if err == nil {
		return wecomapp.InboundReservation{ID: row.ID, Inserted: true, State: row.State}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return wecomapp.InboundReservation{}, err
	}
	existing, err := queries.GetWeComContactInboxByKey(ctx, wecomdb.GetWeComContactInboxByKeyParams{Source: string(envelope.Source), SourceKey: envelope.SourceKey})
	if err != nil {
		return wecomapp.InboundReservation{}, err
	}
	return wecomapp.InboundReservation{ID: existing.ID, State: existing.State}, nil
}

func (repository *InboundRepository) MarkInboundQueued(ctx context.Context, inboxID, jobID int64) error {
	if repository == nil || inboxID <= 0 || jobID <= 0 {
		return wecomapp.ErrInboundProcess
	}
	queries, err := inboundQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.MarkWeComContactInboxQueued(ctx, wecomdb.MarkWeComContactInboxQueuedParams{ID: inboxID, RiverJobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInboundClaimLost
	}
	if err != nil || row.ID != inboxID || row.RiverJobID.Int64 != jobID || !row.RiverJobID.Valid {
		return errors.Join(ErrInboundStore, err)
	}
	return nil
}

func (repository *InboundRepository) ClaimInbound(ctx context.Context, inboxID int64, owner string, expiresAt time.Time) (wecomapp.InboundRecord, error) {
	if repository == nil || inboxID <= 0 || owner == "" || strings.TrimSpace(owner) != owner || expiresAt.IsZero() {
		return wecomapp.InboundRecord{}, wecomapp.ErrInvalidInboundService
	}
	queries, err := inboundQueries(ctx)
	if err != nil {
		return wecomapp.InboundRecord{}, err
	}
	row, err := queries.ClaimWeComContactInbox(ctx, wecomdb.ClaimWeComContactInboxParams{
		ID: inboxID, LeaseOwner: owner, LeaseExpiresAt: pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return wecomapp.InboundRecord{}, wecomapp.ErrInboundAlreadyDone
	}
	if err != nil {
		return wecomapp.InboundRecord{}, err
	}
	return inboundRecord(row), nil
}

func (repository *InboundRepository) CompleteInbound(ctx context.Context, inboxID, fence int64, state string) error {
	if repository == nil || inboxID <= 0 || fence <= 0 || !validTerminalState(state) {
		return wecomapp.ErrInboundProcess
	}
	queries, err := inboundQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.CompleteWeComContactInbox(ctx, wecomdb.CompleteWeComContactInboxParams{ID: inboxID, LeaseFence: fence, State: state})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInboundClaimLost
	}
	if err != nil || row.ID != inboxID {
		return errors.Join(ErrInboundStore, err)
	}
	return nil
}

func (repository *InboundRepository) FailInbound(ctx context.Context, inboxID, fence int64, lastError string) error {
	if repository == nil || inboxID <= 0 || fence <= 0 || lastError == "" || len(lastError) > 2048 {
		return wecomapp.ErrInboundProcess
	}
	queries, err := inboundQueries(ctx)
	if err != nil {
		return err
	}
	row, err := queries.FailWeComContactInbox(ctx, wecomdb.FailWeComContactInboxParams{ID: inboxID, LeaseFence: fence, LastError: lastError})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInboundClaimLost
	}
	if err != nil || row.ID != inboxID {
		return errors.Join(ErrInboundStore, err)
	}
	return nil
}

func (repository *InboundRepository) ReserveSyncFact(ctx context.Context, fact wecomapp.SyncHandoff) (wecomapp.InboundReservation, error) {
	return repository.ReserveInbound(ctx, wecomapp.InboundEnvelope{
		Source: wecomapp.InboundSourceSync, SourceKey: fact.FactID, CorpID: fact.CorpID,
		EventType: "wecom.external_contact.sync_observed", ExternalUserID: fact.ExternalUserID,
		RawPayload: fact.Payload, OccurredAt: fact.OccurredAt, InitialState: "pending",
	})
}

func inboundQueries(ctx context.Context) (*wecomdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return wecomdb.New(tx), nil
}

func validEnvelope(envelope wecomapp.InboundEnvelope) bool {
	if envelope.Source != wecomapp.InboundSourceCallback && envelope.Source != wecomapp.InboundSourceSync ||
		envelope.SourceKey == "" || len(envelope.SourceKey) > 512 || strings.TrimSpace(envelope.SourceKey) != envelope.SourceKey ||
		envelope.CorpID == "" || len(envelope.CorpID) > 256 || strings.TrimSpace(envelope.CorpID) != envelope.CorpID ||
		envelope.EventType == "" || len(envelope.EventType) > 256 || strings.TrimSpace(envelope.EventType) != envelope.EventType ||
		len(envelope.ExternalUserID) > 1024 || len(envelope.RawPayload) == 0 || len(envelope.RawPayload) > 1<<20 || envelope.OccurredAt.IsZero() {
		return false
	}
	if envelope.ExternalContact != nil {
		fact := envelope.ExternalContact
		if envelope.Source != wecomapp.InboundSourceCallback || fact.CorpID != envelope.CorpID || fact.ExternalUserID != envelope.ExternalUserID || fact.OccurredAt.UTC() != envelope.OccurredAt.UTC() || fact.EventType() != envelope.EventType || fact.ChangeType == "" || (fact.WelcomeCodePresent != (fact.WelcomeCodeDigest != "")) || !validOptionalDigest(fact.WelcomeCodeDigest) || !validOptionalDigest(fact.SourceDigest) || !validOptionalDigest(fact.FailReasonDigest) {
			return false
		}
	}
	return true
}

func initialState(envelope wecomapp.InboundEnvelope) string {
	if envelope.InitialState == "skipped" {
		return "skipped"
	}
	return "pending"
}

func validTerminalState(state string) bool {
	switch state {
	case "processed", "pending_identity", "conflict", "skipped":
		return true
	default:
		return false
	}
}

func inboundRecord(row wecomdb.ClaimWeComContactInboxRow) wecomapp.InboundRecord {
	result := wecomapp.InboundRecord{
		ID: row.ID, Source: wecomapp.InboundSource(row.Source), SourceKey: row.SourceKey,
		CorpID: row.CorpID, EventType: row.EventType, ExternalUserID: row.ExternalUserid,
		RawPayload: append([]byte(nil), row.RawPayload...), OccurredAt: row.OccurredAt.Time.UTC(),
		State: row.State, AttemptCount: row.AttemptCount, LeaseFence: row.LeaseFence,
		LeaseOwner: row.LeaseOwner,
	}
	if row.RiverJobID.Valid {
		result.RiverJobID = row.RiverJobID.Int64
	}
	if row.ExternalContactChangeType != "" {
		result.ExternalContact = &wecomapp.ExternalContactCallbackFact{CorpID: row.CorpID, ChangeType: row.ExternalContactChangeType, ExternalUserID: row.ExternalUserid, UserID: row.ExternalContactWecomUserid, State: row.ExternalContactState, OccurredAt: row.OccurredAt.Time.UTC(), WelcomeCodePresent: row.ExternalContactWelcomePresent, WelcomeCodeDigest: row.ExternalContactWelcomeDigest, SourceDigest: row.ExternalContactSourceDigest, FailReasonDigest: row.ExternalContactFailReasonDigest}
	}
	return result
}

func externalContactChangeType(envelope wecomapp.InboundEnvelope) string {
	if envelope.ExternalContact == nil {
		return ""
	}
	return envelope.ExternalContact.ChangeType
}
func externalContactUserID(envelope wecomapp.InboundEnvelope) string {
	if envelope.ExternalContact == nil {
		return ""
	}
	return envelope.ExternalContact.UserID
}
func externalContactState(envelope wecomapp.InboundEnvelope) string {
	if envelope.ExternalContact == nil {
		return ""
	}
	return envelope.ExternalContact.State
}
func externalContactWelcomePresent(envelope wecomapp.InboundEnvelope) bool {
	return envelope.ExternalContact != nil && envelope.ExternalContact.WelcomeCodePresent
}
func externalContactWelcomeDigest(envelope wecomapp.InboundEnvelope) string {
	if envelope.ExternalContact == nil {
		return ""
	}
	return envelope.ExternalContact.WelcomeCodeDigest
}
func externalContactSourceDigest(envelope wecomapp.InboundEnvelope) string {
	if envelope.ExternalContact == nil {
		return ""
	}
	return envelope.ExternalContact.SourceDigest
}
func externalContactFailReasonDigest(envelope wecomapp.InboundEnvelope) string {
	if envelope.ExternalContact == nil {
		return ""
	}
	return envelope.ExternalContact.FailReasonDigest
}
func validOptionalDigest(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[7:] {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

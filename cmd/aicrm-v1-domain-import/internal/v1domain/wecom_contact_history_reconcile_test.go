package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
)

type wecomContactHistoryReconcileReader struct {
	event  contactport.HistoricalWeComExternalContactEventLog
	follow contactport.HistoricalWeComExternalContactFollowUser
	err    error
}

func (r *wecomContactHistoryReconcileReader) GetHistoricalWeComExternalContactEventLog(_ context.Context, id int64) (contactport.HistoricalWeComExternalContactEventLog, error) {
	if r.err != nil || id != r.event.ID {
		return contactport.HistoricalWeComExternalContactEventLog{}, firstWeComReconcileError(r.err)
	}
	return r.event, nil
}
func (r *wecomContactHistoryReconcileReader) GetHistoricalWeComExternalContactFollowUser(_ context.Context, id int64) (contactport.HistoricalWeComExternalContactFollowUser, error) {
	if r.err != nil || id != r.follow.ID {
		return contactport.HistoricalWeComExternalContactFollowUser{}, firstWeComReconcileError(r.err)
	}
	return r.follow, nil
}
func (*wecomContactHistoryReconcileReader) ListHistoricalWeComExternalContactEventLog(context.Context, contactport.WeComContactHistoryQuery) ([]contactport.HistoricalWeComExternalContactEventLog, int64, error) {
	return nil, 0, nil
}
func (*wecomContactHistoryReconcileReader) ListHistoricalWeComExternalContactFollowUser(context.Context, contactport.WeComContactHistoryQuery) ([]contactport.HistoricalWeComExternalContactFollowUser, int64, error) {
	return nil, 0, nil
}
func firstWeComReconcileError(err error) error {
	if err != nil {
		return err
	}
	return contactport.ErrWeComContactHistoryUnavailable
}

func TestVerifyWeComContactHistoryRowBindsCompleteTargets(t *testing.T) {
	for _, follow := range []bool{false, true} {
		reader, row := wecomContactHistoryReconcileFixture(t, follow)
		proof, err := verifyWeComContactHistoryRow(context.Background(), reader, row)
		if err != nil || proof != "history_only:"+hex.EncodeToString(row.TargetDigest) {
			t.Fatalf("valid follow=%t proof=%q err=%v", follow, proof, err)
		}
		for name, mutate := range map[string]func(*wecomContactHistoryReconcileReader, *reconciliationRow){
			"source-key":    func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
			"payload":       func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) { row.PayloadDigest[0]++ },
			"field":         func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) { row.FieldDigest[0]++ },
			"target-digest": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) { row.TargetDigest[0]++ },
			"wrong-domain": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) {
				value := "identity"
				row.TargetDomain = &value
			},
			"wrong-table": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) {
				value := "customers"
				row.TargetTable = &value
			},
			"missing-id": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) { row.TargetID = nil },
			"noncanonical-id": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) {
				value := "0071"
				row.TargetID = &value
			},
			"nonpositive-id": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) {
				value := "0"
				row.TargetID = &value
			},
			"missing-row": func(_ *wecomContactHistoryReconcileReader, row *reconciliationRow) {
				value := "99"
				row.TargetID = &value
			},
			"actual-id": func(reader *wecomContactHistoryReconcileReader, _ *reconciliationRow) {
				reader.event.ID++
				reader.follow.ID++
			},
			"private-target": func(reader *wecomContactHistoryReconcileReader, _ *reconciliationRow) {
				reader.event.IdentitySyncResponseDigest[0]++
				reader.follow.State += "-drift"
			},
			"read-error": func(reader *wecomContactHistoryReconcileReader, _ *reconciliationRow) {
				reader.err = errors.New("unavailable")
			},
		} {
			t.Run(strconv.FormatBool(follow)+"/"+name, func(t *testing.T) {
				candidateReader, candidateRow := wecomContactHistoryReconcileFixture(t, follow)
				mutate(candidateReader, &candidateRow)
				if _, err := verifyWeComContactHistoryRow(context.Background(), candidateReader, candidateRow); !errors.Is(err, ErrConflict) {
					t.Fatalf("drift %s accepted: %v", name, err)
				}
			})
		}
	}
}

func TestVerifyWeComContactHistoryRowRejectsTypedNilReader(t *testing.T) {
	_, row := wecomContactHistoryReconcileFixture(t, false)
	var typedNil *contactstore.WeComContactHistoryReader
	if _, err := verifyWeComContactHistoryRow(context.Background(), typedNil, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("typed nil err=%v", err)
	}
}

func TestReconcileWeComContactHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileWeComContactHistory(context.Background(), nil, "v1-wecom-contact-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong version accepted")
	}
}

func wecomContactHistoryReconcileFixture(t *testing.T, follow bool) (*wecomContactHistoryReconcileReader, reconciliationRow) {
	t.Helper()
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	digest := func(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
	reader := &wecomContactHistoryReconcileReader{
		event:  contactport.HistoricalWeComExternalContactEventLog{ID: 71, SourceKeyDigest: digest(1), SourcePayloadDigest: digest(2), SourceFieldDigest: digest(3), SourceID: -7, CorpIDDigest: digest(4), ExternalUserIDDigest: digest(5), UserIDDigest: digest(6), EventKeyDigest: digest(7), PayloadXMLDigest: digest(8), PayloadJSONDigest: digest(9), RetryCount: -2, ErrorMessageDigest: digest(10), CreatedAt: at, UpdatedAt: at.Add(-time.Second), IdentitySyncErrorCodeDigest: digest(11), IdentitySyncErrorMessageDigest: digest(12), IdentitySyncResponseDigest: digest(13)},
		follow: contactport.HistoricalWeComExternalContactFollowUser{ID: 72, SourceKeyDigest: digest(14), SourcePayloadDigest: digest(15), SourceFieldDigest: digest(16), SourceID: 0, CorpIDDigest: digest(17), ExternalUserIDDigest: digest(18), UserIDDigest: digest(19), RemarkDigest: digest(20), DescriptionDigest: digest(21), State: "private-state", OperUserIDDigest: digest(22), RawFollowUserDigest: digest(23), FirstSeenAt: at, LastSeenAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)},
	}
	domain, table, id, source := "contact", "contact_v1_wecom_event_log_history", "71", "public/wecom_external_contact_event_logs"
	key, payload, field := reader.event.SourceKeyDigest, reader.event.SourcePayloadDigest, reader.event.SourceFieldDigest
	targetDigest, err := contactapp.HistoricalWeComExternalContactEventLogDigest(reader.event)
	if follow {
		table, id, source = "contact_v1_wecom_follow_user_history", "72", "public/wecom_external_contact_follow_users"
		key, payload, field = reader.follow.SourceKeyDigest, reader.follow.SourcePayloadDigest, reader.follow.SourceFieldDigest
		targetDigest, err = contactapp.HistoricalWeComExternalContactFollowUserDigest(reader.follow)
	}
	if err != nil {
		t.Fatal(err)
	}
	return reader, reconciliationRow{TableID: source, SourceKeyDigest: key[:], PayloadDigest: payload[:], FieldDigest: field[:], TargetDomain: &domain, TargetTable: &table, TargetID: &id, TargetDigest: targetDigest[:]}
}

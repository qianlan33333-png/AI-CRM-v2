package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type messageHistoryReaderFake struct {
	value wecomport.HistoricalMessage
	err   error
}

func (fake messageHistoryReaderFake) GetHistoricalMessage(_ context.Context, id int64) (wecomport.HistoricalMessage, error) {
	if fake.err != nil {
		return wecomport.HistoricalMessage{}, fake.err
	}
	if id != fake.value.ID {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryConflict
	}
	return fake.value, nil
}

func (messageHistoryReaderFake) ListHistoricalMessages(context.Context, wecomport.MessageHistoryQuery) ([]wecomport.HistoricalMessage, int64, error) {
	return nil, 0, nil
}

func messageHistoryReconcileFixture(t *testing.T) (messageHistoryReaderFake, reconciliationRow) {
	t.Helper()
	sequence := int64(-4)
	value := wecomport.HistoricalMessage{ID: 71, SourceID: 9007199254740993, Sequence: &sequence, ChatType: "private", MessageType: "text",
		OriginalSendTime: "2026-08-27 13:36:01", SendTimeBasis: "civil_unzoned", CreatedAt: time.Date(2026, 8, 27, 13, 36, 2, 123456000, time.UTC),
		SourcePayloadDigest: sha256.Sum256([]byte("sealed-message-payload"))}
	digest, err := wecomapp.HistoricalMessageDigest(value)
	if err != nil {
		t.Fatal("fixture_digest_failed")
	}
	domain, table, targetID := "wecom", messageHistoryTargetTable, "71"
	return messageHistoryReaderFake{value: value}, reconciliationRow{TableID: messageHistoryTableID, PayloadDigest: value.SourcePayloadDigest[:],
		TargetDomain: &domain, TargetTable: &table, TargetID: &targetID, TargetDigest: digest[:]}
}

func TestVerifyMessageHistoryRowChecksCompleteReadOnlyProjection(t *testing.T) {
	reader, row := messageHistoryReconcileFixture(t)
	got, err := verifyMessageHistoryRow(context.Background(), reader, row)
	wantDigest, _ := wecomapp.HistoricalMessageDigest(reader.value)
	if err != nil || got != "history_only:"+hex.EncodeToString(wantDigest[:]) {
		t.Fatal("civil_null_projection_rejected")
	}
	for name, mutate := range map[string]func(*messageHistoryReaderFake, *reconciliationRow){
		"source-id": func(reader *messageHistoryReaderFake, _ *reconciliationRow) { reader.value.SourceID++ },
		"payload":   func(_ *messageHistoryReaderFake, row *reconciliationRow) { row.PayloadDigest[0]++ },
		"domain": func(_ *messageHistoryReaderFake, row *reconciliationRow) {
			value := "contact"
			row.TargetDomain = &value
		},
		"table": func(_ *messageHistoryReaderFake, row *reconciliationRow) {
			value := "messages"
			row.TargetTable = &value
		},
		"id": func(_ *messageHistoryReaderFake, row *reconciliationRow) {
			value := "0071"
			row.TargetID = &value
		},
		"target-digest": func(_ *messageHistoryReaderFake, row *reconciliationRow) { row.TargetDigest[0]++ },
	} {
		t.Run(name, func(t *testing.T) {
			candidate, candidateRow := messageHistoryReconcileFixture(t)
			mutate(&candidate, &candidateRow)
			if _, err := verifyMessageHistoryRow(context.Background(), candidate, candidateRow); !errors.Is(err, ErrConflict) {
				t.Fatal("projection_drift_accepted")
			}
		})
	}
}

func TestReconcileMessageHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileMessageHistory(context.Background(), nil, "v1-message-history-a2", "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong_version_reached_database")
	}
}

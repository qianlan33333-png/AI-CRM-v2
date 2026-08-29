package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	memberusage "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcmemberusagehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestHXCMemberUsageTerminalReaderVerifiesOneBoundedBatch(t *testing.T) {
	values := []memberusage.SourceEnvelope{memberUsageTerminalEnvelope(1), memberUsageTerminalEnvelope(2)}
	fake := &memberUsageTerminalFake{recordRows: memberUsageTerminalRecords("run", values), receiptRows: memberUsageTerminalReceipts("run", values)}
	reader := &HXCMemberUsageTerminalReader{source: fake}
	if err := reader.VerifyArchiveTerminals(context.Background(), "run", values); err != nil {
		t.Fatal(err)
	}
	if fake.recordCalls != 1 || fake.receiptCalls != 1 || fake.recordKeyCount != len(values) || fake.receiptKeyCount != len(values) {
		t.Fatalf("expected one bounded query per table: records=%d/%d receipts=%d/%d", fake.recordCalls, fake.recordKeyCount, fake.receiptCalls, fake.receiptKeyCount)
	}
}

func TestHXCMemberUsageTerminalReaderFailsClosedOnTerminalDrift(t *testing.T) {
	values := []memberusage.SourceEnvelope{memberUsageTerminalEnvelope(1), memberUsageTerminalEnvelope(2)}
	tests := []struct {
		name   string
		values []memberusage.SourceEnvelope
		mutate func(*memberUsageTerminalFake)
	}{
		{name: "missing archive", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows = fake.recordRows[:1] }},
		{name: "duplicate archive", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows = append(fake.recordRows, fake.recordRows[0]) }},
		{name: "archive scope", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows[0].AdapterID = "wrong" }},
		{name: "archive run", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows[0].RunID = "wrong" }},
		{name: "archive key", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows[0].SourceKeyDigest[0]++ }},
		{name: "archive payload", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows[0].PayloadDigest[0]++ }},
		{name: "archive field", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows[0].FieldDigest[0]++ }},
		{name: "archive ordinal", mutate: func(fake *memberUsageTerminalFake) { fake.recordRows[0].SourceOrdinal++ }},
		{name: "missing receipt", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows = fake.receiptRows[:1] }},
		{name: "duplicate receipt", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows = append(fake.receiptRows, fake.receiptRows[0]) }},
		{name: "receipt scope", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].TableID = "public/wrong" }},
		{name: "receipt run", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].RunID = "wrong" }},
		{name: "receipt key", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].SourceKeyDigest[0]++ }},
		{name: "receipt payload", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].PayloadDigest[0]++ }},
		{name: "receipt field", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].FieldDigest[0]++ }},
		{name: "receipt disposition", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].Disposition = "import" }},
		{name: "receipt operation", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].Operation = "unexpected" }},
		{name: "receipt mapping", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].MappingDigest[0]++ }},
		{name: "receipt policy", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].PolicyDigest[0]++ }},
		{name: "receipt mutation", mutate: func(fake *memberUsageTerminalFake) { fake.receiptRows[0].MutationDigest[0]++ }},
		{name: "duplicate input", values: []memberusage.SourceEnvelope{values[0], values[0]}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			batch := test.values
			if batch == nil {
				batch = values
			}
			fake := memberUsageTerminalFake{recordRows: memberUsageTerminalRecords("run", batch), receiptRows: memberUsageTerminalReceipts("run", batch)}
			if test.mutate != nil {
				test.mutate(&fake)
			}
			err := (&HXCMemberUsageTerminalReader{source: &fake}).VerifyArchiveTerminals(context.Background(), "run", batch)
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("terminal drift err=%v", err)
			}
		})
	}
}

func TestHXCMemberUsageTerminalReaderRejectsInvalidBatchBeforeRead(t *testing.T) {
	values := make([]memberusage.SourceEnvelope, memberusage.StreamBatchSize+1)
	for index := range values {
		values[index] = memberUsageTerminalEnvelope(int64(index + 1))
	}
	fake := memberUsageTerminalFake{}
	err := (&HXCMemberUsageTerminalReader{source: &fake}).VerifyArchiveTerminals(context.Background(), "run", values)
	if !errors.Is(err, ErrInvalidScope) || fake.recordCalls != 0 || fake.receiptCalls != 0 {
		t.Fatalf("oversized batch err=%v record=%d receipt=%d", err, fake.recordCalls, fake.receiptCalls)
	}
}

type memberUsageTerminalFake struct {
	recordRows                []hxcMemberUsageArchiveRecord
	receiptRows               []hxcMemberUsageArchiveReceipt
	recordCalls, receiptCalls int
	recordKeyCount            int
	receiptKeyCount           int
	recordErr, receiptErr     error
}

func (fake *memberUsageTerminalFake) archiveRecords(_ context.Context, _ string, keys [][]byte) ([]hxcMemberUsageArchiveRecord, error) {
	fake.recordCalls++
	fake.recordKeyCount = len(keys)
	return fake.recordRows, fake.recordErr
}

func (fake *memberUsageTerminalFake) archiveReceipts(_ context.Context, _ string, keys [][]byte) ([]hxcMemberUsageArchiveReceipt, error) {
	fake.receiptCalls++
	fake.receiptKeyCount = len(keys)
	return fake.receiptRows, fake.receiptErr
}

func memberUsageTerminalEnvelope(ordinal int64) memberusage.SourceEnvelope {
	var key, payload, field [sha256.Size]byte
	key[0], payload[0], field[0] = byte(ordinal), byte(ordinal+32), byte(ordinal+64)
	return memberusage.SourceEnvelope{SourceOrdinal: ordinal, SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field}
}

func memberUsageTerminalRecords(run string, values []memberusage.SourceEnvelope) []hxcMemberUsageArchiveRecord {
	result := make([]hxcMemberUsageArchiveRecord, 0, len(values))
	for _, value := range values {
		var schema [sha256.Size]byte
		schema[0] = byte(value.SourceOrdinal + 96)
		result = append(result, hxcMemberUsageArchiveRecord{RunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: memberusage.MemberUsageProjectionTableID, SourceOrdinal: value.SourceOrdinal, SourceKeyDigest: value.SourceKeyHMAC, PayloadDigest: value.PayloadHMAC, FieldDigest: value.FieldHMAC, SchemaDigest: schema})
	}
	return result
}

func memberUsageTerminalReceipts(run string, values []memberusage.SourceEnvelope) []hxcMemberUsageArchiveReceipt {
	records := memberUsageTerminalRecords(run, values)
	result := make([]hxcMemberUsageArchiveReceipt, 0, len(values))
	for index, value := range values {
		schema := records[index].SchemaDigest
		result = append(result, hxcMemberUsageArchiveReceipt{RunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: memberusage.MemberUsageProjectionTableID, SourceKeyDigest: value.SourceKeyHMAC, PayloadDigest: value.PayloadHMAC, FieldDigest: value.FieldHMAC, Disposition: "archive", MappingDigest: hxcMemberUsageArchiveMappingDigest(schema), PolicyDigest: v1archive.ArchivePolicyDigest(), MutationDigest: hxcMemberUsageArchiveMutationDigest(value, schema)})
	}
	return result
}

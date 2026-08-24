package migration

import (
	"errors"
	"testing"
)

type receiptStore struct{ rows map[string]RowReceipt }

func (s *receiptStore) FindRowReceipt(r RowReceipt) (RowReceipt, bool, error) {
	got, ok := s.rows[r.SourceTable+string(r.SourceKey)]
	return got, ok, nil
}

func (s *receiptStore) AppendRowReceipt(r RowReceipt) error {
	s.rows[r.SourceTable+string(r.SourceKey)] = r
	return nil
}

func TestRecordRowReplayAndPayloadDrift(t *testing.T) {
	s := &receiptStore{rows: map[string]RowReceipt{}}
	r := RowReceipt{SourceTable: "crm_user_identity", SourceKey: make([]byte, 32), PayloadHMAC: make([]byte, 32), Disposition: "imported"}
	owned, err := RecordRow(s, r)
	if err != nil || !owned {
		t.Fatalf("first = %v, %v", owned, err)
	}
	owned, err = RecordRow(s, r)
	if err != nil || owned {
		t.Fatalf("replay = %v, %v", owned, err)
	}
	drift := RowReceipt{SourceTable: r.SourceTable, SourceKey: append([]byte(nil), r.SourceKey...), PayloadHMAC: make([]byte, 32), Disposition: r.Disposition}
	drift.PayloadHMAC[0] = 1
	if _, err := RecordRow(s, drift); !errors.Is(err, ErrSourcePayloadDrift) {
		t.Fatalf("drift = %v", err)
	}
}

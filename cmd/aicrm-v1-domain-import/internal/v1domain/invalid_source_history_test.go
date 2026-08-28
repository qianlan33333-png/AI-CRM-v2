package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	invalidhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1invalidsourcehistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestInvalidSourceJournalBridgeRejectsWrongKindAndMalformedReceipt(t *testing.T) {
	journal, err := NewJournal(Scope{ImportVersion: InvalidSourceHistoryVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: invalidhistory.ImageLibraryTable, TargetDomain: "media", TargetTable: "media_v1_invalid_asset_history"})
	if err != nil {
		t.Fatal(err)
	}
	bridge := invalidSourceJournal{journal: journal, kind: "invalid_asset"}
	if _, _, err := bridge.LoadInvalidSourceHistory(context.Background(), "invalid_link", strings.Repeat("1", 64)); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong kind load: %v", err)
	}
	digest := [sha256.Size]byte{1}
	valid := contactport.InvalidSourceHistoryReceipt{Kind: "invalid_asset", SourceIdentifier: strings.Repeat("1", 64), PayloadDigest: digest, TargetDigest: digest, TargetID: 1}
	for name, value := range map[string]contactport.InvalidSourceHistoryReceipt{
		"wrong kind": func() contactport.InvalidSourceHistoryReceipt { v := valid; v.Kind = "invalid_link"; return v }(),
		"bad source": func() contactport.InvalidSourceHistoryReceipt { v := valid; v.SourceIdentifier = "bad"; return v }(),
		"zero payload": func() contactport.InvalidSourceHistoryReceipt {
			v := valid
			v.PayloadDigest = [sha256.Size]byte{}
			return v
		}(),
		"zero target": func() contactport.InvalidSourceHistoryReceipt {
			v := valid
			v.TargetDigest = [sha256.Size]byte{}
			return v
		}(),
		"replayed": func() contactport.InvalidSourceHistoryReceipt { v := valid; v.Replayed = true; return v }(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := bridge.RecordInvalidSourceHistory(context.Background(), value); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("malformed receipt reached journal: %v", err)
			}
		})
	}
}

func TestRunInvalidSourceHistoryRejectsMissingRequiredParameters(t *testing.T) {
	pool := newFakeInvalidSourcePool()
	archive := invalidSourceNoopArchive{}
	key := []byte("01234567890123456789012345678901")
	for name, input := range map[string]struct {
		ctx     context.Context
		pool    *pgxpool.Pool
		archive invalidhistory.ArchiveSource
		run     string
		key     []byte
	}{
		"nil context": {nil, pool, archive, "archive-run", key},
		"nil pool":    {context.Background(), nil, archive, "archive-run", key},
		"nil archive": {context.Background(), pool, nil, "archive-run", key},
		"empty run":   {context.Background(), pool, archive, "", key},
		"short key":   {context.Background(), pool, archive, "archive-run", key[:31]},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := RunInvalidSourceHistory(input.ctx, input.pool, input.archive, input.run, input.key, false); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("parameters accepted: %v", err)
			}
		})
	}
}

type invalidSourceNoopArchive struct{}

func (invalidSourceNoopArchive) EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error {
	return nil
}
func newFakeInvalidSourcePool() *pgxpool.Pool { return &pgxpool.Pool{} }

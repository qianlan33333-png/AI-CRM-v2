package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1radarhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type radarClickTestTx struct{}

type radarClickArchive struct{ rows []v1archive.ArchivedRow }

func (a radarClickArchive) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	if table != v1radarhistory.ClickTableID {
		return v1domain.ErrInvalidScope
	}
	for _, row := range a.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type radarClickRuntime struct {
	terminals map[string]v1domain.TerminalReceipt
	values    map[int64]radarport.HistoricalRadarClick
	writes    int
}

func newRadarClickRuntime() *radarClickRuntime {
	return &radarClickRuntime{terminals: map[string]v1domain.TerminalReceipt{}, values: map[int64]radarport.HistoricalRadarClick{}}
}

func (r *radarClickRuntime) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, radarClickTestTx{}, true))
}
func (r *radarClickRuntime) LoadRadarClickHistoryTerminal(ctx context.Context, source string) (v1domain.TerminalReceipt, bool, error) {
	if ctx.Value(radarClickTestTx{}) != true {
		return v1domain.TerminalReceipt{}, false, v1domain.ErrInvalidScope
	}
	v, ok := r.terminals[source]
	return v, ok, nil
}
func (r *radarClickRuntime) RecordRadarClickHistoryTerminal(ctx context.Context, value v1domain.TerminalReceipt) error {
	if ctx.Value(radarClickTestTx{}) != true {
		return v1domain.ErrInvalidScope
	}
	key := v1domain.SourceIdentifier(value.SourceKeyDigest)
	if prior, ok := r.terminals[key]; ok && !reflect.DeepEqual(prior, value) {
		return v1domain.ErrConflict
	}
	r.terminals[key] = value
	return nil
}
func (r *radarClickRuntime) LoadRadarClickHistory(ctx context.Context, kind, source string) (radarport.RadarClickHistoryReceipt, bool, error) {
	if kind != v1domain.RadarClickHistoryKind {
		return radarport.RadarClickHistoryReceipt{}, false, v1domain.ErrInvalidScope
	}
	v, found, err := r.LoadRadarClickHistoryTerminal(ctx, source)
	if err != nil || !found {
		return radarport.RadarClickHistoryReceipt{}, found, err
	}
	if v.Disposition != "import" {
		return radarport.RadarClickHistoryReceipt{}, false, v1domain.ErrConflict
	}
	id, err := strconv.ParseInt(v.TargetID, 10, 64)
	if err != nil {
		return radarport.RadarClickHistoryReceipt{}, false, err
	}
	return radarport.RadarClickHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: v.PayloadDigest, TargetID: id, TargetDigest: v.TargetDigest}, true, nil
}
func (r *radarClickRuntime) RecordRadarClickHistory(context.Context, radarport.RadarClickHistoryReceipt) error {
	return v1domain.ErrInvalidScope
}
func (r *radarClickRuntime) ImportHistoricalRadarClick(ctx context.Context, source string, value radarport.HistoricalRadarClick) (radarport.RadarClickHistoryReceipt, error) {
	if ctx.Value(radarClickTestTx{}) != true {
		return radarport.RadarClickHistoryReceipt{}, radarport.ErrRadarClickHistoryUnavailable
	}
	if terminal, found, _ := r.LoadRadarClickHistoryTerminal(ctx, source); found {
		id, err := strconv.ParseInt(terminal.TargetID, 10, 64)
		if err != nil || !reflect.DeepEqual(r.values[id], valueWithRadarClickID(value, id)) {
			return radarport.RadarClickHistoryReceipt{}, radarport.ErrRadarClickHistoryConflict
		}
		return radarport.RadarClickHistoryReceipt{Kind: v1domain.RadarClickHistoryKind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest, Replayed: true}, nil
	}
	r.writes++
	id := int64(len(r.values) + 1)
	value = valueWithRadarClickID(value, id)
	r.values[id] = value
	digest := sha256.Sum256([]byte("radar/" + source))
	receipt := radarport.RadarClickHistoryReceipt{Kind: v1domain.RadarClickHistoryKind, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: id, TargetDigest: digest}
	if err := r.RecordRadarClickHistoryTerminal(ctx, v1domain.TerminalReceipt{SourceKeyDigest: value.SourceKeyDigest, PayloadDigest: value.SourcePayloadDigest, Disposition: "import", TargetID: fmt.Sprint(id), TargetDigest: digest}); err != nil {
		return radarport.RadarClickHistoryReceipt{}, err
	}
	return receipt, nil
}
func valueWithRadarClickID(value radarport.HistoricalRadarClick, id int64) radarport.HistoricalRadarClick {
	value.ID = id
	return value
}

type radarClickReferences struct{ err error }

func (r radarClickReferences) ResolveHistoricalRadarClick(_ context.Context, _ v1radarhistory.ClickFact) (*int64, *int64, error) {
	if r.err != nil {
		return nil, nil, r.err
	}
	link, customer := int64(41), int64(71)
	return &link, &customer, nil
}

func TestRadarClickHistoryImporterPreservesFactsAndReplays(t *testing.T) {
	rows := make([]v1archive.ArchivedRow, 1735)
	for index := range rows {
		rows[index] = radarClickRow(t, int64(index+1))
	}
	runtime := newRadarClickRuntime()
	importer, err := v1domain.NewRadarClickHistoryImporter(radarClickArchive{rows}, runtime, runtime, radarClickReferences{}, runtime)
	if err != nil {
		t.Fatal(err)
	}
	first, err := importer.Import(context.Background(), "run")
	if err != nil || first != (v1domain.RadarClickHistoryImportResult{Imported: 1735}) || runtime.writes != 1735 {
		t.Fatal("radar_click_import_failed")
	}
	value := runtime.values[1]
	if value.SourceID != 1 || value.LinkSourceID != 9 || value.RadarLinkID == nil || *value.RadarLinkID != 41 || value.CustomerID == nil || *value.CustomerID != 71 || value.CreatedAt != time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC) || value.UnionIDDigest == ([32]byte{}) || value.QueryParamsDigest == ([32]byte{}) {
		t.Fatal("radar_click_fact_changed")
	}
	encoded, _ := json.Marshal(value)
	if string(encoded) == "" || containsAny(string(encoded), "union-private", "ip-private", "query-private") {
		t.Fatal("radar_click_private_fact_serialized")
	}
	second, err := importer.Import(context.Background(), "run")
	if err != nil || second != (v1domain.RadarClickHistoryImportResult{Imported: 1735, Replayed: 1735}) || runtime.writes != 1735 {
		t.Fatal("radar_click_replay_failed")
	}
}

func TestRadarClickHistoryImporterFailsBeforeTargetOnReferenceDrift(t *testing.T) {
	runtime := newRadarClickRuntime()
	rows := make([]v1archive.ArchivedRow, 1735)
	for index := range rows {
		rows[index] = radarClickRow(t, int64(index+1))
	}
	importer, err := v1domain.NewRadarClickHistoryImporter(radarClickArchive{rows}, runtime, runtime, radarClickReferences{err: v1domain.ErrConflict}, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "run"); !errors.Is(err, v1domain.ErrConflict) || runtime.writes != 0 || len(runtime.terminals) != 0 {
		t.Fatal("radar_click_reference_drift_not_closed")
	}
}

func radarClickRow(t *testing.T, id int64) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "link_id": int64(9), "code": "code", "stage": "legacy", "openid": "openid-private", "unionid": "union-private", "external_userid": "external-private", "source_channel": "source", "campaign_id": "campaign", "staff_id": "staff", "user_agent": "agent", "ip": "ip-private", "created_at": "2026-08-28T09:02:03.123456+08:00", "target_type_snapshot": "url", "person_id": "person", "ip_hash": "hash", "source_channel_snapshot": "snapshot", "campaign_id_snapshot": "campaign-snapshot", "staff_id_snapshot": "staff-snapshot", "referer": "referrer", "query_params_json": map[string]any{"q": "query-private"}, "error_code": ""})
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: v1radarhistory.ClickTableID, SourceOrdinal: id, SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("radar/key/%d", id))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(fmt.Sprintf("radar/field/%d", id))), Payload: payload}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

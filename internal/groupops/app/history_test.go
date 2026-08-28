package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

type historicalFake struct {
	ctx                                   context.Context
	plan                                  groupopsport.HistoricalPlan
	directory                             groupopsport.HistoricalDirectory
	group                                 groupopsport.HistoricalGroup
	node                                  groupopsport.HistoricalNode
	receipts                              map[string]groupopsport.HistoricalReceipt
	creates, gets, loads, records         int
	loadErr, recordErr, createErr, getErr error
	createdID                             int64
	corruptCreate                         bool
}

func (f *historicalFake) check(ctx context.Context) {
	if ctx != f.ctx {
		panic("caller context changed")
	}
}
func (f *historicalFake) LoadGroupOpsHistory(ctx context.Context, kind, source string) (groupopsport.HistoricalReceipt, bool, error) {
	f.check(ctx)
	f.loads++
	r, found := f.receipts[kind+":"+source]
	return r, found, f.loadErr
}
func (f *historicalFake) RecordGroupOpsHistory(ctx context.Context, kind string, r groupopsport.HistoricalReceipt) error {
	f.check(ctx)
	f.records++
	if f.recordErr == nil {
		f.receipts[kind+":"+r.SourceIdentifier] = r
	}
	return f.recordErr
}
func (f *historicalFake) CreateHistoricalPlan(ctx context.Context, r groupopsport.HistoricalPlan) (groupopsport.HistoricalPlan, error) {
	f.check(ctx)
	f.creates++
	r.ID = f.createdID
	if f.corruptCreate {
		r.OriginalStatus += "changed"
	}
	f.plan = r
	return r, f.createErr
}
func (f *historicalFake) GetHistoricalPlan(ctx context.Context, _ int64) (groupopsport.HistoricalPlan, error) {
	f.check(ctx)
	f.gets++
	return f.plan, f.getErr
}
func (f *historicalFake) CreateHistoricalDirectory(ctx context.Context, r groupopsport.HistoricalDirectory) (groupopsport.HistoricalDirectory, error) {
	f.check(ctx)
	f.creates++
	r.ID = f.createdID
	f.directory = r
	return r, f.createErr
}
func (f *historicalFake) GetHistoricalDirectory(ctx context.Context, _ int64) (groupopsport.HistoricalDirectory, error) {
	f.check(ctx)
	f.gets++
	return f.directory, f.getErr
}
func (f *historicalFake) CreateHistoricalGroup(ctx context.Context, r groupopsport.HistoricalGroup) (groupopsport.HistoricalGroup, error) {
	f.check(ctx)
	f.creates++
	r.ID = f.createdID
	f.group = r
	return r, f.createErr
}
func (f *historicalFake) GetHistoricalGroup(ctx context.Context, _ int64) (groupopsport.HistoricalGroup, error) {
	f.check(ctx)
	f.gets++
	return f.group, f.getErr
}
func (f *historicalFake) CreateHistoricalNode(ctx context.Context, r groupopsport.HistoricalNode) (groupopsport.HistoricalNode, error) {
	f.check(ctx)
	f.creates++
	r.ID = f.createdID
	f.node = r
	return r, f.createErr
}
func (f *historicalFake) GetHistoricalNode(ctx context.Context, _ int64) (groupopsport.HistoricalNode, error) {
	f.check(ctx)
	f.gets++
	return f.node, f.getErr
}

func historyPointer[T any](value T) *T { return &value }
func historicalPlanFixture() groupopsport.HistoricalPlan {
	at := time.Date(2026, 8, 28, 8, 0, 0, 123456789, time.FixedZone("source", 8*60*60))
	return groupopsport.HistoricalPlan{Plan: groupopsport.Plan{Name: "历史计划", Status: groupopsport.PlanArchived, Revision: 1, CreatedBy: 2, UpdatedBy: 2, CreatedAt: at, UpdatedAt: at.Add(time.Hour)}, SourcePlanID: 7, SourceCode: " old-code ", PlanType: " old-type ", OriginalStatus: "active", OwnerStaffID: nil, ArchivedAt: &at}
}
func historicalDirectoryFixture(snapshot bool) groupopsport.HistoricalDirectory {
	r := groupopsport.HistoricalDirectory{SourceKind: "group_chats", SourceID: historyPointer(int64(9)), ChatReference: " old-chat ", MemberCount: historyPointer(int32(0)), OriginalStatus: "", RecordedAt: historicalPlanFixture().CreatedAt}
	if snapshot {
		r.SourceKind, r.SourceID, r.MemberCount = "wecom_group_chat_snapshots", nil, nil
		r.InternalMemberCount, r.ExternalMemberCount = historyPointer(int32(0)), historyPointer(int32(31))
		r.DisplayName, r.OwnerName = historyPointer(""), historyPointer(" 原成员名 ")
	}
	return r
}
func historicalGroupFixture() groupopsport.HistoricalGroup {
	at := historicalPlanFixture().CreatedAt
	return groupopsport.HistoricalGroup{SourceGroupID: 12, SourcePlanID: 7, PlanID: 100, ChatReference: "chat-ref", DisplayName: " 原群名 ", OriginalStatus: "removed", CreatedAt: at, RemovedAt: historyPointer(at.Add(-time.Hour))}
}
func historicalNodeFixture() groupopsport.HistoricalNode {
	at := historicalPlanFixture().CreatedAt
	return groupopsport.HistoricalNode{SourceNodeID: 21, SourcePlanID: 7, PlanID: 100, DayIndex: 0, TriggerTime: "原时间标签 8:05", SortOrder: 0, OriginalStatus: "paused", ContentPackage: json.RawMessage(`{"z":9007199254740993,"a":{"y":null,"x":[2,1]}}`), CreatedAt: at, UpdatedAt: at.Add(-time.Hour)}
}
func historicalWriterFixture(t *testing.T) (*HistoricalWriter, *historicalFake) {
	t.Helper()
	f := &historicalFake{ctx: context.WithValue(context.Background(), struct{}{}, "caller transaction"), receipts: map[string]groupopsport.HistoricalReceipt{}, createdID: 500, plan: historicalPlanFixture()}
	f.plan.ID = 100
	w, err := NewHistoricalWriter(f, f)
	if err != nil {
		t.Fatal(err)
	}
	return w, f
}

func TestHistoricalWriterCreateReplayAndDrift(t *testing.T) {
	for _, kind := range []string{"plans", "group_chats", "snapshots", "groups", "nodes"} {
		t.Run(kind, func(t *testing.T) {
			w, f := historicalWriterFixture(t)
			plan, directory, group, node := historicalPlanFixture(), historicalDirectoryFixture(kind == "snapshots"), historicalGroupFixture(), historicalNodeFixture()
			call := func(payload [32]byte) (groupopsport.HistoricalReceipt, error) {
				switch kind {
				case "plans":
					return w.ImportPlan(f.ctx, "source-hmac", payload, plan)
				case "group_chats", "snapshots":
					return w.ImportDirectory(f.ctx, "source-hmac", payload, directory)
				case "groups":
					return w.ImportGroup(f.ctx, "source-hmac", payload, group)
				default:
					return w.ImportNode(f.ctx, "source-hmac", payload, node)
				}
			}
			payload := sha256.Sum256([]byte("encrypted-source-fact"))
			first, err := call(payload)
			if err != nil || first.TargetID != 500 || first.Replayed || first.TargetDigest == [32]byte{} || f.creates != 1 || f.records != 1 || f.receipts[kind+":source-hmac"] != first {
				t.Fatalf("first import: %+v, %v", first, err)
			}
			if kind == "nodes" {
				f.node.ContentPackage = json.RawMessage(` { "a": { "x": [2,1], "y": null }, "z": 9007199254740993 } `)
			}
			gets := f.gets
			replay, err := call(payload)
			if err != nil || !replay.Replayed || replay.TargetDigest != first.TargetDigest || f.creates != 1 || f.records != 1 || f.gets <= gets {
				t.Fatalf("actual-target replay: %+v, %v", replay, err)
			}
			if _, err := call(sha256.Sum256([]byte("changed"))); !errors.Is(err, groupopsport.ErrHistoryConflict) {
				t.Fatalf("payload drift: %v", err)
			}
			switch kind {
			case "plans":
				f.plan.OriginalStatus += "changed"
			case "group_chats", "snapshots":
				f.directory.OriginalStatus += "changed"
			case "groups":
				f.group.OriginalStatus += "changed"
			case "nodes":
				f.node.ContentPackage = json.RawMessage(`{"z":9007199254740992,"a":{"y":null,"x":[2,1]}}`)
			}
			if _, err := call(payload); !errors.Is(err, groupopsport.ErrHistoryConflict) {
				t.Fatalf("actual target drift: %v", err)
			}
			if f.creates != 1 || f.records != 1 {
				t.Fatal("replay performed writes")
			}
		})
	}
}

func TestHistoricalWriterRejectsWrongParentOnCreateAndReplay(t *testing.T) {
	for _, child := range []string{"group", "node"} {
		for _, field := range []string{"id", "source", "status", "revision"} {
			for _, replay := range []bool{false, true} {
				t.Run(child+"/"+field+"/"+map[bool]string{false: "create", true: "replay"}[replay], func(t *testing.T) {
					w, f := historicalWriterFixture(t)
					call := func() (groupopsport.HistoricalReceipt, error) {
						if child == "group" {
							return w.ImportGroup(f.ctx, "source", [32]byte{1}, historicalGroupFixture())
						}
						return w.ImportNode(f.ctx, "source", [32]byte{1}, historicalNodeFixture())
					}
					if replay {
						if _, err := call(); err != nil {
							t.Fatal(err)
						}
					}
					switch field {
					case "id":
						f.plan.ID++
					case "source":
						f.plan.SourcePlanID++
					case "status":
						f.plan.Status = groupopsport.PlanActive
					case "revision":
						f.plan.Revision++
					}
					creates, records := f.creates, f.records
					if _, err := call(); !errors.Is(err, groupopsport.ErrHistoryConflict) {
						t.Fatalf("wrong parent: %v", err)
					}
					if f.creates != creates || f.records != records {
						t.Fatal("wrong parent caused writes")
					}
				})
			}
		}
	}
}

func TestHistoricalWriterPreservesNullsCivilLabelsAndJSONNumbers(t *testing.T) {
	w, f := historicalWriterFixture(t)
	plan := historicalPlanFixture()
	if _, err := w.ImportPlan(f.ctx, "plan", [32]byte{1}, plan); err != nil {
		t.Fatal(err)
	}
	if f.plan.SourceCode != plan.SourceCode || f.plan.OwnerStaffID != nil || f.plan.CreatedAt.Location() != time.UTC || f.plan.CreatedAt.Nanosecond() != 123456000 || !plan.CreatedAt.Equal(time.Date(2026, 8, 28, 8, 0, 0, 123456789, time.FixedZone("source", 8*60*60))) {
		t.Fatal("plan facts or caller time altered")
	}
	f.plan.ID = 100
	for _, snapshot := range []bool{false, true} {
		r := historicalDirectoryFixture(snapshot)
		if _, err := w.ImportDirectory(f.ctx, r.SourceKind, [32]byte{1}, r); err != nil {
			t.Fatal(err)
		}
		want := r
		want.ID = 500
		want.RecordedAt = historicalTime(want.RecordedAt)
		if !reflect.DeepEqual(f.directory, want) {
			t.Fatal("nullable directory fields changed")
		}
	}
	r := historicalNodeFixture()
	if _, err := w.ImportNode(f.ctx, "node", [32]byte{1}, r); err != nil {
		t.Fatal(err)
	}
	if f.node.DayIndex != 0 || f.node.SortOrder != 0 || f.node.TriggerTime != r.TriggerTime || !f.node.UpdatedAt.Before(f.node.CreatedAt) || !strings.Contains(string(f.node.ContentPackage), "9007199254740993") || !strings.Contains(string(f.node.ContentPackage), `"x":[2,1]`) {
		t.Fatal("historical schedule or JSON facts changed")
	}
	if _, err := w.ImportGroup(f.ctx, "group", [32]byte{1}, historicalGroupFixture()); err != nil || !f.group.RemovedAt.Before(f.group.CreatedAt) {
		t.Fatalf("historical dates rejected or rewritten: %v", err)
	}
}

func TestHistoricalWriterInputAndDependencyFailures(t *testing.T) {
	w, f := historicalWriterFixture(t)
	var absent *historicalFake
	for _, test := range []struct {
		store   groupopsport.HistoricalStore
		journal groupopsport.HistoricalJournal
	}{{nil, f}, {f, nil}, {absent, f}, {f, absent}} {
		if _, err := NewHistoricalWriter(test.store, test.journal); !errors.Is(err, groupopsport.ErrHistoryUnavailable) {
			t.Fatalf("nil dependency: %v", err)
		}
	}
	for _, mutate := range []func(*groupopsport.HistoricalPlan){func(r *groupopsport.HistoricalPlan) { r.ID = 1 }, func(r *groupopsport.HistoricalPlan) { r.SourcePlanID = 0 }, func(r *groupopsport.HistoricalPlan) { r.Status = groupopsport.PlanActive }, func(r *groupopsport.HistoricalPlan) { r.Revision = 2 }, func(r *groupopsport.HistoricalPlan) { r.UpdatedBy = 3 }, func(r *groupopsport.HistoricalPlan) { r.Name = " padded " }, func(r *groupopsport.HistoricalPlan) { r.Name = strings.Repeat("名", 129) }, func(r *groupopsport.HistoricalPlan) { r.Name = "x\x00" }, func(r *groupopsport.HistoricalPlan) { r.UpdatedAt = r.CreatedAt.Add(-time.Hour) }, func(r *groupopsport.HistoricalPlan) { r.OwnerStaffID = historyPointer(int64(0)) }} {
		r := historicalPlanFixture()
		mutate(&r)
		if _, err := w.ImportPlan(f.ctx, "source", [32]byte{1}, r); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
			t.Fatalf("invalid plan: %v", err)
		}
	}
	for _, mutate := range []func(*groupopsport.HistoricalDirectory){func(r *groupopsport.HistoricalDirectory) { r.SourceKind = "other" }, func(r *groupopsport.HistoricalDirectory) { r.SourceID = nil }, func(r *groupopsport.HistoricalDirectory) { r.MemberCount = nil }, func(r *groupopsport.HistoricalDirectory) { r.MemberCount = historyPointer(int32(-1)) }, func(r *groupopsport.HistoricalDirectory) { r.OwnerName = historyPointer("") }, func(r *groupopsport.HistoricalDirectory) { r.InternalMemberCount = historyPointer(int32(0)) }} {
		r := historicalDirectoryFixture(false)
		mutate(&r)
		if _, err := w.ImportDirectory(f.ctx, "source", [32]byte{1}, r); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
			t.Fatalf("invalid directory: %v", err)
		}
	}
	for _, raw := range []string{"null", "[]", "{", "{} {}"} {
		r := historicalNodeFixture()
		r.ContentPackage = json.RawMessage(raw)
		if _, err := w.ImportNode(f.ctx, "source", [32]byte{1}, r); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
			t.Fatalf("invalid JSON %q: %v", raw, err)
		}
		if HistoricalNodeTargetDigest(r) != [32]byte{} {
			t.Fatal("invalid JSON got a valid digest")
		}
	}
	for _, source := range []string{"", " padded ", "bad\x00"} {
		if _, err := w.ImportPlan(f.ctx, source, [32]byte{1}, historicalPlanFixture()); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
			t.Fatalf("invalid source: %v", err)
		}
	}
	if _, err := w.ImportPlan(f.ctx, "source", [32]byte{}, historicalPlanFixture()); !errors.Is(err, groupopsport.ErrHistoryInvalid) {
		t.Fatal(err)
	}
	if _, err := w.ImportPlan(nil, "source", [32]byte{1}, historicalPlanFixture()); !errors.Is(err, groupopsport.ErrHistoryUnavailable) {
		t.Fatal(err)
	}
	var nilWriter *HistoricalWriter
	if _, err := nilWriter.ImportPlan(f.ctx, "source", [32]byte{1}, historicalPlanFixture()); !errors.Is(err, groupopsport.ErrHistoryUnavailable) {
		t.Fatal(err)
	}
	if f.loads != 0 || f.creates != 0 || f.records != 0 {
		t.Fatal("invalid input touched journal or store")
	}
}

func TestHistoricalWriterErrorsAndReceiptDrift(t *testing.T) {
	for _, stage := range []string{"load", "create", "record", "get"} {
		for _, want := range []error{groupopsport.ErrHistoryConflict, groupopsport.ErrHistoryInvalid, groupopsport.ErrHistoryUnavailable} {
			t.Run(stage+"/"+want.Error(), func(t *testing.T) {
				w, f := historicalWriterFixture(t)
				call := func() (groupopsport.HistoricalReceipt, error) {
					return w.ImportPlan(f.ctx, "source", [32]byte{1}, historicalPlanFixture())
				}
				if stage == "get" {
					if _, err := call(); err != nil {
						t.Fatal(err)
					}
				}
				failure := want
				if want == groupopsport.ErrHistoryUnavailable {
					failure = errors.New("database details must not escape")
				}
				switch stage {
				case "load":
					f.loadErr = failure
				case "create":
					f.createErr = failure
				case "record":
					f.recordErr = failure
				case "get":
					f.getErr = failure
				}
				r, err := call()
				if err != want || r != (groupopsport.HistoricalReceipt{}) {
					t.Fatalf("error/result: %+v %v", r, err)
				}
			})
		}
	}
	for _, field := range []string{"source", "payload", "id", "digest", "actual_id", "input"} {
		t.Run(field, func(t *testing.T) {
			w, f := historicalWriterFixture(t)
			value := historicalPlanFixture()
			if _, err := w.ImportPlan(f.ctx, "source", [32]byte{1}, value); err != nil {
				t.Fatal(err)
			}
			r := f.receipts["plans:source"]
			switch field {
			case "source":
				r.SourceIdentifier = "other"
			case "payload":
				r.PayloadDigest = [32]byte{2}
			case "id":
				r.TargetID = 0
			case "digest":
				r.TargetDigest = [32]byte{}
			case "actual_id":
				f.plan.ID++
			case "input":
				value.OriginalStatus += "changed"
			}
			f.receipts["plans:source"] = r
			if _, err := w.ImportPlan(f.ctx, "source", [32]byte{1}, value); !errors.Is(err, groupopsport.ErrHistoryConflict) {
				t.Fatalf("receipt drift: %v", err)
			}
			if f.creates != 1 || f.records != 1 {
				t.Fatal("drift caused writes")
			}
		})
	}
	for _, wrongID := range []bool{false, true} {
		w, f := historicalWriterFixture(t)
		if wrongID {
			f.createdID = 0
		} else {
			f.corruptCreate = true
		}
		if _, err := w.ImportPlan(f.ctx, "source", [32]byte{1}, historicalPlanFixture()); !errors.Is(err, groupopsport.ErrHistoryConflict) || f.records != 0 {
			t.Fatalf("bad create result: %v", err)
		}
	}
}

func TestHistoricalTargetDigestsIncludeEveryField(t *testing.T) {
	// Mutate each exported leaf field, including embedded Plan and nullable fields.
	tests := []struct {
		name   string
		value  any
		digest func(any) [32]byte
	}{
		{"plan", historicalPlanFixture(), func(v any) [32]byte { return HistoricalPlanTargetDigest(v.(groupopsport.HistoricalPlan)) }},
		{"directory", historicalDirectoryFixture(true), func(v any) [32]byte { return HistoricalDirectoryTargetDigest(v.(groupopsport.HistoricalDirectory)) }},
		{"group", historicalGroupFixture(), func(v any) [32]byte { return HistoricalGroupTargetDigest(v.(groupopsport.HistoricalGroup)) }},
		{"node", historicalNodeFixture(), func(v any) [32]byte { return HistoricalNodeTargetDigest(v.(groupopsport.HistoricalNode)) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var visit func(reflect.Value, []int)
			visit = func(v reflect.Value, index []int) {
				for i := 0; i < v.NumField(); i++ {
					path := append(append([]int{}, index...), i)
					field := v.Field(i)
					if field.Kind() == reflect.Struct && field.Type() != reflect.TypeOf(time.Time{}) {
						visit(field, path)
						continue
					}
					changed := reflect.New(reflect.TypeOf(test.value)).Elem()
					changed.Set(reflect.ValueOf(test.value))
					target := changed.FieldByIndex(path)
					switch field.Kind() {
					case reflect.String:
						target.SetString(field.String() + "changed")
					case reflect.Int, reflect.Int32, reflect.Int64:
						target.SetInt(field.Int() + 1)
					case reflect.Pointer:
						if field.IsNil() {
							target.Set(reflect.New(field.Type().Elem()))
						} else {
							target.SetZero()
						}
					case reflect.Slice:
						target.SetBytes([]byte(`{"changed":true}`))
					case reflect.Struct:
						target.Set(reflect.ValueOf(field.Interface().(time.Time).Add(time.Microsecond)))
					default:
						t.Fatalf("uncovered field %s", v.Type().Field(i).Name)
					}
					if test.digest(changed.Interface()) == test.digest(test.value) {
						t.Fatalf("digest omitted field %s", v.Type().Field(i).Name)
					}
				}
			}
			visit(reflect.ValueOf(test.value), nil)
		})
	}
	node := historicalNodeFixture()
	equivalent := node
	equivalent.CreatedAt = historicalTime(node.CreatedAt)
	equivalent.UpdatedAt = historicalTime(node.UpdatedAt)
	equivalent.ContentPackage = json.RawMessage(`{ "a":{"x":[2,1],"y":null}, "z":9007199254740993 }`)
	if HistoricalNodeTargetDigest(node) != HistoricalNodeTargetDigest(equivalent) {
		t.Fatal("JSONB ordering/whitespace or UTC microseconds changed digest")
	}
	equivalent.ContentPackage = json.RawMessage(`{"a":{"x":[1,2],"y":null},"z":9007199254740993}`)
	if HistoricalNodeTargetDigest(node) == HistoricalNodeTargetDigest(equivalent) {
		t.Fatal("JSON array order lost")
	}
}

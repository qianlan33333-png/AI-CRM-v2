package v1domain

// This file is the bounded runtime bridge for the two immutable Audience
// activity sources.  It deliberately owns no current-audience dependency.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"reflect"

	activity "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audienceactivityhistory"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const AudienceActivityHistoryImportVersion = "v1-audience-activity-history-a1"

type AudienceActivityArchiveReady interface {
	VerifyAudienceActivityArchiveReady(context.Context, string) error
}
type AudienceActivityWriter interface {
	WriteRun(context.Context, string, [32]byte, segment.HistoricalAudienceActivityRun) (segment.AudienceActivityHistoryReceipt, error)
	WriteMemberEvent(context.Context, string, [32]byte, segment.HistoricalAudienceActivityMemberEvent) (segment.AudienceActivityHistoryReceipt, error)
}
type AudienceActivityTargets interface {
	GetHistoricalAudienceActivityRun(context.Context, int64) (segment.HistoricalAudienceActivityRun, error)
	GetHistoricalAudienceActivityMemberEvent(context.Context, int64) (segment.HistoricalAudienceActivityMemberEvent, error)
}
type AudienceActivityTerminal struct {
	Version, ArchiveRunID, TableID, Kind  string
	SourceKeyHMAC, PayloadHMAC, FieldHMAC [32]byte
	Disposition, Reason                   string
	TargetID                              int64
	TargetDigest                          [32]byte
}
type AudienceActivityJournal interface {
	LoadAudienceActivityTerminal(context.Context, string, [32]byte) (AudienceActivityTerminal, bool, error)
	RecordAudienceActivityTerminal(context.Context, AudienceActivityTerminal) error
}
type AudienceActivitySeal struct {
	Version, ArchiveRunID                                                                            string
	SelectedSourceCount, ReceiptCount, ImportedCount, ArchivedCount, QuarantinedCount, VerifiedCount int64
	ComparisonDigest                                                                                 [32]byte
}
type AudienceActivityReconciliation interface {
	LoadAudienceActivityReconciliationSeal(context.Context, string, string) (AudienceActivitySeal, bool, error)
	RecordAudienceActivityReconciliationSeal(context.Context, AudienceActivitySeal) error
}

type AudienceActivityHistoryImporter struct {
	ready   AudienceActivityArchiveReady
	archive activity.ArchiveSource
	uow     UnitOfWork
	writer  AudienceActivityWriter
	refs    segment.AudienceActivityHistoryReferences
	targets AudienceActivityTargets
	journal AudienceActivityJournal
	seals   AudienceActivityReconciliation
}
type AudienceActivityHistoryImportResult struct{ PackageRuns, MemberEvents, Imported, Quarantined, Replayed int64 }
type AudienceActivityHistoryReconciliationResult struct {
	SelectedSourceCount, ReceiptCount, ImportedCount, ArchivedCount, QuarantinedCount, VerifiedCount int64
	ComparisonDigest                                                                                 [32]byte
	Replayed                                                                                         bool
}

func NewAudienceActivityHistoryImporter(ready AudienceActivityArchiveReady, archive activity.ArchiveSource, uow UnitOfWork, writer AudienceActivityWriter, refs segment.AudienceActivityHistoryReferences, targets AudienceActivityTargets, journal AudienceActivityJournal, seals AudienceActivityReconciliation) (*AudienceActivityHistoryImporter, error) {
	if nilAudienceActivity(ready) || nilAudienceActivity(archive) || nilAudienceActivity(uow) || nilAudienceActivity(writer) || nilAudienceActivity(refs) || nilAudienceActivity(targets) || nilAudienceActivity(journal) || nilAudienceActivity(seals) {
		return nil, ErrInvalidScope
	}
	return &AudienceActivityHistoryImporter{ready, archive, uow, writer, refs, targets, journal, seals}, nil
}
func (i *AudienceActivityHistoryImporter) valid(ctx context.Context, run string, key []byte) bool {
	return i != nil && ctx != nil && ctx.Err() == nil && run != "" && len(key) >= sha256.Size && !nilAudienceActivity(i.ready) && !nilAudienceActivity(i.archive) && !nilAudienceActivity(i.uow) && !nilAudienceActivity(i.writer) && !nilAudienceActivity(i.refs) && !nilAudienceActivity(i.targets) && !nilAudienceActivity(i.journal) && !nilAudienceActivity(i.seals)
}
func (i *AudienceActivityHistoryImporter) Import(ctx context.Context, run string, key []byte) (AudienceActivityHistoryImportResult, error) {
	if !i.valid(ctx, run, key) {
		return AudienceActivityHistoryImportResult{}, ErrInvalidScope
	}
	if err := i.uow.Within(ctx, func(tx context.Context) error { return i.ready.VerifyAudienceActivityArchiveReady(tx, run) }); err != nil {
		return AudienceActivityHistoryImportResult{}, err
	}
	// Complete envelope/shape authentication precedes the first target batch.
	if _, err := activity.Stream(ctx, i.archive, run, key, audienceActivityNoop{}, nil); err != nil {
		return AudienceActivityHistoryImportResult{}, err
	}
	c := &audienceActivityImportConsumer{importer: i, run: run}
	if _, err := activity.Stream(ctx, i.archive, run, key, audienceActivityNoop{}, c); err != nil {
		return AudienceActivityHistoryImportResult{}, err
	}
	return c.result, nil
}
func (i *AudienceActivityHistoryImporter) Reconcile(ctx context.Context, run string, key []byte) (AudienceActivityHistoryReconciliationResult, error) {
	if !i.valid(ctx, run, key) {
		return AudienceActivityHistoryReconciliationResult{}, ErrInvalidScope
	}
	if err := i.uow.Within(ctx, func(tx context.Context) error { return i.ready.VerifyAudienceActivityArchiveReady(tx, run) }); err != nil {
		return AudienceActivityHistoryReconciliationResult{}, err
	}
	c := &audienceActivityReconcileConsumer{importer: i, run: run, hash: sha256.New()}
	if _, err := activity.Stream(ctx, i.archive, run, key, audienceActivityNoop{}, c); err != nil {
		return AudienceActivityHistoryReconciliationResult{}, err
	}
	return c.seal(ctx)
}

type audienceActivityImportConsumer struct {
	importer *AudienceActivityHistoryImporter
	run      string
	result   AudienceActivityHistoryImportResult
}

func (c *audienceActivityImportConsumer) ConsumeAudienceActivityPackageRunBatch(ctx context.Context, rows []activity.PackageRunResult) error {
	return c.consumeRuns(ctx, rows)
}
func (c *audienceActivityImportConsumer) ConsumeAudienceActivityMemberEventBatch(ctx context.Context, rows []activity.MemberEventResult) error {
	return c.consumeEvents(ctx, rows)
}
func (c *audienceActivityImportConsumer) consumeRuns(ctx context.Context, rows []activity.PackageRunResult) error {
	if c == nil || c.importer == nil || len(rows) == 0 || len(rows) > activity.FixedBatchSize {
		return ErrInvalidScope
	}
	var imported, quarantined, replayed int64
	err := c.importer.uow.Within(ctx, func(tx context.Context) error {
		for _, row := range rows {
			if row.Disposition == activity.DispositionQuarantine {
				r, e := c.importer.record(tx, c.run, activity.PackageRunsTableID, "package_runs", row.Source, row.Disposition, row.Reason, 0, [32]byte{})
				if e != nil {
					return e
				}
				if r {
					replayed++
				} else {
					quarantined++
				}
				continue
			}
			if row.Disposition != activity.DispositionCandidate || row.Fact == nil {
				return ErrConflict
			}
			value, err := c.importer.runValue(tx, row.Source, *row.Fact)
			if err != nil && audienceActivityUnresolved(err) {
				r, e := c.importer.record(tx, c.run, activity.PackageRunsTableID, "package_runs", row.Source, activity.DispositionQuarantine, "audience_activity_run_parent_unresolved", 0, [32]byte{})
				if e != nil {
					return e
				}
				if r {
					replayed++
				} else {
					quarantined++
				}
				continue
			}
			if err != nil {
				return err
			}
			receipt, e := c.importer.writer.WriteRun(tx, hex.EncodeToString(value.SourceKeyDigest[:]), value.SourcePayloadDigest, value)
			if e != nil {
				return e
			}
			if e = validateAudienceActivityReceipt(receipt, value.SourceKeyDigest, value.SourcePayloadDigest); e != nil {
				return e
			}
			terminal, found, e := c.importer.journal.LoadAudienceActivityTerminal(tx, AudienceActivityHistoryImportVersion, row.Source.SourceKeyHMAC)
			if e != nil || !found || !audienceActivityTerminalMatches(terminal, c.run, activity.PackageRunsTableID, "package_runs", row.Source, activity.DispositionCandidate, "") || terminal.TargetID != receipt.TargetID || terminal.TargetDigest != receipt.TargetDigest {
				return ErrConflict
			}
			if receipt.Replayed {
				replayed++
			} else {
				imported++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.result.PackageRuns += int64(len(rows))
	c.result.Imported += imported
	c.result.Quarantined += quarantined
	c.result.Replayed += replayed
	return nil
}
func (c *audienceActivityImportConsumer) consumeEvents(ctx context.Context, rows []activity.MemberEventResult) error {
	if c == nil || c.importer == nil || len(rows) == 0 || len(rows) > activity.FixedBatchSize {
		return ErrInvalidScope
	}
	var imported, quarantined, replayed int64
	err := c.importer.uow.Within(ctx, func(tx context.Context) error {
		for _, row := range rows {
			if row.Disposition == activity.DispositionQuarantine {
				r, e := c.importer.record(tx, c.run, activity.MemberEventsTableID, "member_events", row.Source, row.Disposition, row.Reason, 0, [32]byte{})
				if e != nil {
					return e
				}
				if r {
					replayed++
				} else {
					quarantined++
				}
				continue
			}
			if row.Disposition != activity.DispositionCandidate || row.Fact == nil {
				return ErrConflict
			}
			value, err := c.importer.eventValue(tx, row.Source, *row.Fact)
			if err != nil && audienceActivityUnresolved(err) {
				r, e := c.importer.record(tx, c.run, activity.MemberEventsTableID, "member_events", row.Source, activity.DispositionQuarantine, "audience_activity_event_parent_unresolved", 0, [32]byte{})
				if e != nil {
					return e
				}
				if r {
					replayed++
				} else {
					quarantined++
				}
				continue
			}
			if err != nil {
				return err
			}
			receipt, e := c.importer.writer.WriteMemberEvent(tx, hex.EncodeToString(value.SourceKeyDigest[:]), value.SourcePayloadDigest, value)
			if e != nil {
				return e
			}
			if e = validateAudienceActivityReceipt(receipt, value.SourceKeyDigest, value.SourcePayloadDigest); e != nil {
				return e
			}
			terminal, found, e := c.importer.journal.LoadAudienceActivityTerminal(tx, AudienceActivityHistoryImportVersion, row.Source.SourceKeyHMAC)
			if e != nil || !found || !audienceActivityTerminalMatches(terminal, c.run, activity.MemberEventsTableID, "member_events", row.Source, activity.DispositionCandidate, "") || terminal.TargetID != receipt.TargetID || terminal.TargetDigest != receipt.TargetDigest {
				return ErrConflict
			}
			if receipt.Replayed {
				replayed++
			} else {
				imported++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.result.MemberEvents += int64(len(rows))
	c.result.Imported += imported
	c.result.Quarantined += quarantined
	c.result.Replayed += replayed
	return nil
}
func (i *AudienceActivityHistoryImporter) runValue(ctx context.Context, source activity.SourceEnvelope, fact activity.PackageRunFact) (segment.HistoricalAudienceActivityRun, error) {
	p, e := i.refs.ResolveAudienceActivityPackage(ctx, fact.PackageSourceID)
	if e != nil {
		return segment.HistoricalAudienceActivityRun{}, e
	}
	var v *int64
	if fact.VersionSourceID != nil {
		x, e := i.refs.ResolveAudienceActivityVersion(ctx, *fact.VersionSourceID)
		if e != nil || x.PackageHistoryID != p.ID {
			return segment.HistoricalAudienceActivityRun{}, ErrConflict
		}
		n := x.ID
		v = &n
	}
	return segment.HistoricalAudienceActivityRun{SourceKeyDigest: source.SourceKeyHMAC, SourcePayloadDigest: source.PayloadHMAC, SourceFieldDigest: source.FieldHMAC, SourceID: fact.SourceID, PackageHistoryID: p.ID, VersionHistoryID: v, RunType: fact.RunType, OriginalStatus: fact.OriginalStatus, RefreshStartedAt: fact.RefreshStartedAt, RefreshFinishedAt: fact.RefreshFinishedAt, LastWatermarkAt: fact.LastWatermarkAt, NextWatermarkAt: fact.NextWatermarkAt, ReturnedCount: fact.ReturnedCount, EnteredCount: fact.EnteredCount, UpdatedCount: fact.UpdatedCount, ExitedCount: fact.ExitedCount, MemberEventCount: fact.MemberEventCount, DurationMS: fact.DurationMS, CreatedAt: fact.CreatedAt, PrivateDigest: [32]byte(fact.PrivateDigest)}, nil
}
func (i *AudienceActivityHistoryImporter) eventValue(ctx context.Context, source activity.SourceEnvelope, fact activity.MemberEventFact) (segment.HistoricalAudienceActivityMemberEvent, error) {
	p, e := i.refs.ResolveAudienceActivityPackage(ctx, fact.PackageSourceID)
	if e != nil {
		return segment.HistoricalAudienceActivityMemberEvent{}, e
	}
	var run *int64
	if fact.RunSourceID != nil {
		x, e := i.refs.ResolveAudienceActivityRun(ctx, *fact.RunSourceID)
		if e != nil || x.PackageHistoryID != p.ID {
			return segment.HistoricalAudienceActivityMemberEvent{}, ErrConflict
		}
		n := x.ID
		run = &n
	}
	var member *int64
	if fact.MemberSourceID != nil {
		x, e := i.refs.ResolveAudienceActivityMember(ctx, *fact.MemberSourceID)
		if e != nil || x.PackageHistoryID != p.ID {
			return segment.HistoricalAudienceActivityMemberEvent{}, ErrConflict
		}
		n := x.ID
		member = &n
	}
	return segment.HistoricalAudienceActivityMemberEvent{SourceKeyDigest: source.SourceKeyHMAC, SourcePayloadDigest: source.PayloadHMAC, SourceFieldDigest: source.FieldHMAC, SourceID: fact.SourceID, PackageHistoryID: p.ID, RunHistoryID: run, MemberHistoryID: member, EventType: fact.EventType, IdentityKind: fact.IdentityKind, OccurredAt: fact.OccurredAt, CreatedAt: fact.CreatedAt, PrivateDigest: [32]byte(fact.PrivateDigest)}, nil
}
func (i *AudienceActivityHistoryImporter) record(ctx context.Context, run, table, kind string, source activity.SourceEnvelope, disposition activity.Disposition, reason string, target int64, digest [32]byte) (bool, error) {
	value := AudienceActivityTerminal{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: run, TableID: table, Kind: kind, SourceKeyHMAC: source.SourceKeyHMAC, PayloadHMAC: source.PayloadHMAC, FieldHMAC: source.FieldHMAC, Disposition: string(disposition), Reason: reason, TargetID: target, TargetDigest: digest}
	if !validAudienceActivityTerminal(value) {
		return false, ErrConflict
	}
	old, found, e := i.journal.LoadAudienceActivityTerminal(ctx, value.Version, value.SourceKeyHMAC)
	if e != nil {
		return false, e
	}
	if found {
		return true, equalAudienceActivityTerminal(old, value)
	}
	if e = i.journal.RecordAudienceActivityTerminal(ctx, value); e != nil {
		return false, e
	}
	return false, nil
}

type audienceActivityReconcileConsumer struct {
	importer *AudienceActivityHistoryImporter
	run      string
	result   AudienceActivityHistoryReconciliationResult
	hash     hash.Hash
}

func (c *audienceActivityReconcileConsumer) ConsumeAudienceActivityPackageRunBatch(ctx context.Context, rows []activity.PackageRunResult) error {
	return c.verifyRuns(ctx, rows)
}
func (c *audienceActivityReconcileConsumer) ConsumeAudienceActivityMemberEventBatch(ctx context.Context, rows []activity.MemberEventResult) error {
	return c.verifyEvents(ctx, rows)
}

func (c *audienceActivityReconcileConsumer) verifyRuns(ctx context.Context, rows []activity.PackageRunResult) error {
	return c.verify(ctx, activity.PackageRunsTableID, "package_runs", len(rows), func(tx context.Context, n int) (activity.SourceEnvelope, activity.Disposition, string, error) {
		r := rows[n]
		if r.Disposition != activity.DispositionCandidate {
			return r.Source, r.Disposition, r.Reason, nil
		}
		if r.Fact == nil {
			return r.Source, activity.DispositionQuarantine, "audience_activity_run_parent_unresolved", nil
		}
		_, err := c.importer.runValue(tx, r.Source, *r.Fact)
		if audienceActivityUnresolved(err) {
			return r.Source, activity.DispositionQuarantine, "audience_activity_run_parent_unresolved", nil
		}
		return r.Source, r.Disposition, r.Reason, err
	}, func(tx context.Context, n int, t AudienceActivityTerminal) error {
		r := rows[n]
		if r.Disposition == activity.DispositionCandidate {
			if r.Fact == nil {
				return ErrConflict
			}
			expected, e := c.importer.runValue(tx, r.Source, *r.Fact)
			if e != nil {
				return ErrConflict
			}
			actual, e := c.importer.targets.GetHistoricalAudienceActivityRun(tx, t.TargetID)
			if e != nil {
				return e
			}
			a, e1 := segmentapp.HistoricalAudienceActivityRunDigest(actual)
			b, e2 := segmentapp.HistoricalAudienceActivityRunDigest(withAudienceActivityRunID(expected, t.TargetID))
			if e1 != nil || e2 != nil || a != b || a != t.TargetDigest {
				return ErrConflict
			}
		}
		return nil
	})
}
func (c *audienceActivityReconcileConsumer) verifyEvents(ctx context.Context, rows []activity.MemberEventResult) error {
	return c.verify(ctx, activity.MemberEventsTableID, "member_events", len(rows), func(tx context.Context, n int) (activity.SourceEnvelope, activity.Disposition, string, error) {
		r := rows[n]
		if r.Disposition != activity.DispositionCandidate {
			return r.Source, r.Disposition, r.Reason, nil
		}
		if r.Fact == nil {
			return r.Source, activity.DispositionQuarantine, "audience_activity_event_parent_unresolved", nil
		}
		_, err := c.importer.eventValue(tx, r.Source, *r.Fact)
		if audienceActivityUnresolved(err) {
			return r.Source, activity.DispositionQuarantine, "audience_activity_event_parent_unresolved", nil
		}
		return r.Source, r.Disposition, r.Reason, err
	}, func(tx context.Context, n int, t AudienceActivityTerminal) error {
		r := rows[n]
		if r.Disposition == activity.DispositionCandidate {
			if r.Fact == nil {
				return ErrConflict
			}
			expected, e := c.importer.eventValue(tx, r.Source, *r.Fact)
			if e != nil {
				return ErrConflict
			}
			actual, e := c.importer.targets.GetHistoricalAudienceActivityMemberEvent(tx, t.TargetID)
			if e != nil {
				return e
			}
			a, e1 := segmentapp.HistoricalAudienceActivityMemberEventDigest(actual)
			b, e2 := segmentapp.HistoricalAudienceActivityMemberEventDigest(withAudienceActivityEventID(expected, t.TargetID))
			if e1 != nil || e2 != nil || a != b || a != t.TargetDigest {
				return ErrConflict
			}
		}
		return nil
	})
}
func (c *audienceActivityReconcileConsumer) verify(ctx context.Context, table, kind string, n int, get func(context.Context, int) (activity.SourceEnvelope, activity.Disposition, string, error), verify func(context.Context, int, AudienceActivityTerminal) error) error {
	if c == nil || c.importer == nil || n == 0 || n > activity.FixedBatchSize {
		return ErrInvalidScope
	}
	var out bytes.Buffer
	var count, imported, quarantined int64
	err := c.importer.uow.Within(ctx, func(tx context.Context) error {
		for j := 0; j < n; j++ {
			source, disp, reason, e := get(tx, j)
			if e != nil {
				return e
			}
			t, found, e := c.importer.journal.LoadAudienceActivityTerminal(tx, AudienceActivityHistoryImportVersion, source.SourceKeyHMAC)
			if e != nil || !found || !audienceActivityTerminalMatches(t, c.run, table, kind, source, disp, reason) {
				return ErrConflict
			}
			if disp == activity.DispositionCandidate {
				if t.TargetID < 1 || t.TargetDigest == ([32]byte{}) {
					return ErrConflict
				}
				if e = verify(tx, j, t); e != nil {
					return e
				}
				imported++
			} else if disp == activity.DispositionQuarantine {
				if t.TargetID != 0 || t.TargetDigest != ([32]byte{}) {
					return ErrConflict
				}
				quarantined++
			} else {
				return ErrConflict
			}
			if e = json.NewEncoder(&out).Encode([]any{table, source.SourceOrdinal, hex.EncodeToString(source.SourceKeyHMAC[:]), hex.EncodeToString(source.PayloadHMAC[:]), hex.EncodeToString(source.FieldHMAC[:]), string(disp), reason, t.TargetID, hex.EncodeToString(t.TargetDigest[:])}); e != nil {
				return e
			}
			count++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := c.hash.Write(out.Bytes()); err != nil {
		return err
	}
	c.result.SelectedSourceCount += count
	c.result.ReceiptCount += count
	c.result.ImportedCount += imported
	c.result.QuarantinedCount += quarantined
	c.result.VerifiedCount += count
	return nil
}
func (c *audienceActivityReconcileConsumer) seal(ctx context.Context) (AudienceActivityHistoryReconciliationResult, error) {
	if c == nil || c.importer == nil || c.hash == nil {
		return AudienceActivityHistoryReconciliationResult{}, ErrConflict
	}
	var d [32]byte
	copy(d[:], c.hash.Sum(nil))
	seal := AudienceActivitySeal{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: c.run, SelectedSourceCount: c.result.SelectedSourceCount, ReceiptCount: c.result.ReceiptCount, ImportedCount: c.result.ImportedCount, ArchivedCount: c.result.ArchivedCount, QuarantinedCount: c.result.QuarantinedCount, VerifiedCount: c.result.VerifiedCount, ComparisonDigest: d}
	if !validAudienceActivitySeal(seal) {
		return AudienceActivityHistoryReconciliationResult{}, ErrConflict
	}
	if err := c.importer.uow.Within(ctx, func(tx context.Context) error {
		old, found, e := c.importer.seals.LoadAudienceActivityReconciliationSeal(tx, seal.Version, seal.ArchiveRunID)
		if e != nil {
			return e
		}
		if found {
			if old != seal {
				return ErrConflict
			}
			c.result.Replayed = true
			return nil
		}
		return c.importer.seals.RecordAudienceActivityReconciliationSeal(tx, seal)
	}); err != nil {
		return AudienceActivityHistoryReconciliationResult{}, err
	}
	c.result.ComparisonDigest = d
	return c.result, nil
}

func validateAudienceActivityReceipt(r segment.AudienceActivityHistoryReceipt, source, payload [32]byte) error {
	if r.SourceIdentifier != hex.EncodeToString(source[:]) || r.PayloadDigest != payload || r.TargetID < 1 || r.TargetDigest == ([32]byte{}) {
		return ErrConflict
	}
	return nil
}
func audienceActivityUnresolved(err error) bool {
	return errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidScope) || errors.Is(err, segment.ErrAudienceActivityHistoryConflict) || errors.Is(err, segment.ErrAudienceActivityHistoryInvalid)
}
func validAudienceActivityTerminal(v AudienceActivityTerminal) bool {
	if v.Version != AudienceActivityHistoryImportVersion || v.ArchiveRunID == "" || v.SourceKeyHMAC == ([32]byte{}) || v.PayloadHMAC == ([32]byte{}) || v.FieldHMAC == ([32]byte{}) || ((v.TableID != activity.PackageRunsTableID || v.Kind != "package_runs") && (v.TableID != activity.MemberEventsTableID || v.Kind != "member_events")) {
		return false
	}
	if v.Disposition == string(activity.DispositionCandidate) {
		return v.Reason == "" && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
	}
	return v.Disposition == string(activity.DispositionQuarantine) && v.Reason != "" && v.TargetID == 0 && v.TargetDigest == ([32]byte{})
}
func audienceActivityTerminalMatches(v AudienceActivityTerminal, run, table, kind string, s activity.SourceEnvelope, d activity.Disposition, reason string) bool {
	return validAudienceActivityTerminal(v) && v.ArchiveRunID == run && v.TableID == table && v.Kind == kind && v.SourceKeyHMAC == s.SourceKeyHMAC && v.PayloadHMAC == s.PayloadHMAC && v.FieldHMAC == s.FieldHMAC && v.Disposition == string(d) && v.Reason == reason
}
func equalAudienceActivityTerminal(a, b AudienceActivityTerminal) error {
	if a == b {
		return nil
	}
	return ErrConflict
}
func validAudienceActivitySeal(v AudienceActivitySeal) bool {
	return v.Version == AudienceActivityHistoryImportVersion && v.ArchiveRunID != "" && v.SelectedSourceCount >= 0 && v.ReceiptCount == v.SelectedSourceCount && v.VerifiedCount == v.ReceiptCount && v.ImportedCount+v.ArchivedCount+v.QuarantinedCount == v.ReceiptCount && v.ComparisonDigest != ([32]byte{})
}
func withAudienceActivityRunID(v segment.HistoricalAudienceActivityRun, id int64) segment.HistoricalAudienceActivityRun {
	v.ID = id
	return v
}
func withAudienceActivityEventID(v segment.HistoricalAudienceActivityMemberEvent, id int64) segment.HistoricalAudienceActivityMemberEvent {
	v.ID = id
	return v
}

type audienceActivityNoop struct{}

func (audienceActivityNoop) VerifyAudienceActivityTerminal(context.Context, string, activity.SourceEnvelope, activity.Disposition, string) error {
	return nil
}
func nilAudienceActivity(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	return (x.Kind() == reflect.Ptr || x.Kind() == reflect.Interface || x.Kind() == reflect.Slice || x.Kind() == reflect.Map || x.Kind() == reflect.Func) && x.IsNil()
}

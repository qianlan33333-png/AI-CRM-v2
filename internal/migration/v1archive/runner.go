package v1archive

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
)

const DefaultAdapterID = "v1_full_archive"

type Result struct {
	Mode           Mode
	Manifest       Manifest
	ManifestDigest [sha256.Size]byte
	Summary        Summary
}

// Execute performs preflight, archive-only import, or reconciliation. It
// never invokes a domain target or a provider queue; target persistence is
// limited to the archive writer supplied by composition.
func Execute(ctx context.Context, config Config, mode Mode, run Run, source Source, target TargetWriter) (Result, error) {
	if ctx == nil || !mode.Valid() || source == nil {
		return Result{}, ErrInvalidConfig
	}
	if mode != ModePreflight {
		if err := config.Validate(); err != nil {
			return Result{}, err
		}
	}
	if mode != ModePreflight && target == nil {
		return Result{}, ErrInvalidConfig
	}
	var result Result
	err := source.WithSnapshot(ctx, func(snapshot Snapshot) error {
		manifest, err := snapshot.Manifest(ctx)
		if err != nil {
			return err
		}
		digest, err := manifest.Digest()
		if err != nil {
			return err
		}
		result.Manifest, result.ManifestDigest = manifest, digest
		if identityTarget, ok := target.(interface {
			Identity(context.Context) (SourceIdentity, error)
		}); ok {
			targetIdentity, identityErr := identityTarget.Identity(ctx)
			if identityErr != nil {
				return identityErr
			}
			if manifest.Source.Equal(targetIdentity) {
				return ErrSameDatabase
			}
		}
		if mode == ModePreflight {
			result.Mode = mode
			return nil
		}
		if run.AdapterID == "" {
			run.AdapterID = DefaultAdapterID
		}
		run.Source = manifest.Source
		run.SnapshotDigest = run.SourceDumpDigest
		run.SchemaDigest = digest
		run.PolicyDigest = ArchivePolicyDigest()
		run.ArchiveKeyVersion = config.ArchiveKeyVersion
		if err = run.Validate(); err != nil {
			return err
		}
		switch mode {
		case ModeFull:
			if err = target.EnsureRun(ctx, run, manifest); err != nil {
				return err
			}
			result.Summary, err = archiveSnapshot(ctx, config, run, snapshot, manifest, target)
			if err != nil {
				return err
			}
			if err = target.Complete(ctx, result.Summary); err != nil {
				return err
			}
		case ModeReconcile:
			stored, found, lookupErr := target.Run(ctx, run.ID)
			if lookupErr != nil {
				return lookupErr
			}
			if !found || stored.AdapterID != run.AdapterID || !stored.Source.Equal(run.Source) || stored.SchemaDigest != run.SchemaDigest || stored.SnapshotDigest != run.SnapshotDigest || stored.PolicyDigest != run.PolicyDigest {
				return ErrRunConflict
			}
			result.Summary, err = summarizeSnapshot(ctx, config, run, snapshot, manifest)
			if err != nil {
				return err
			}
			if err = target.Reconcile(ctx, result.Summary); err != nil {
				return err
			}
		}
		result.Mode = mode
		return nil
	})
	return result, err
}

func archiveSnapshot(ctx context.Context, config Config, run Run, snapshot Snapshot, manifest Manifest, target TargetWriter) (Summary, error) {
	summaries := make([]TableSummary, 0, len(manifest.Tables))
	for _, table := range manifest.Tables {
		accumulator := newSummaryAccumulator(table.Name)
		batch := make([]Record, 0, config.BatchSize)
		var ordinal int64
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if err := target.WriteBatch(ctx, batch); err != nil {
				return err
			}
			batch = batch[:0]
			return nil
		}
		err := snapshot.EachRow(ctx, table, func(sourceKeyJSON, payloadJSON []byte) error {
			ordinal++
			record, err := ArchiveRecord(config, run, table, ordinal, sourceKeyJSON, payloadJSON)
			if err != nil {
				return err
			}
			if err = accumulator.Add(record); err != nil {
				return err
			}
			batch = append(batch, record)
			if len(batch) == config.BatchSize {
				return flush()
			}
			return nil
		})
		if err == nil {
			err = flush()
		}
		if err != nil {
			return Summary{}, err
		}
		if ordinal != table.RowCount {
			return Summary{}, fmt.Errorf("%w: public.%s count changed inside snapshot", ErrInvalidManifest, table.Name)
		}
		summaries = append(summaries, accumulator.Summary(run.ID))
	}
	summary := Summary{RunID: run.ID, Tables: summaries}
	if err := summary.Validate(); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func summarizeSnapshot(ctx context.Context, config Config, run Run, snapshot Snapshot, manifest Manifest) (Summary, error) {
	summaries := make([]TableSummary, 0, len(manifest.Tables))
	for _, table := range manifest.Tables {
		accumulator := newSummaryAccumulator(table.Name)
		var ordinal int64
		err := snapshot.EachRow(ctx, table, func(sourceKeyJSON, payloadJSON []byte) error {
			ordinal++
			record, err := ArchiveRecord(config, run, table, ordinal, sourceKeyJSON, payloadJSON)
			if err != nil {
				return err
			}
			return accumulator.Add(record)
		})
		if err != nil {
			return Summary{}, err
		}
		if ordinal != table.RowCount {
			return Summary{}, fmt.Errorf("%w: public.%s count changed inside snapshot", ErrInvalidManifest, table.Name)
		}
		summaries = append(summaries, accumulator.Summary(run.ID))
	}
	summary := Summary{RunID: run.ID, Tables: summaries}
	if err := summary.Validate(); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

type summaryAccumulator struct {
	table string
	count int64
	hash  hash.Hash
}

func newSummaryAccumulator(table string) *summaryAccumulator {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/v1archive/summary/v1\x00" + table + "\x00"))
	return &summaryAccumulator{table: table, hash: hash}
}

func (accumulator *summaryAccumulator) Add(record Record) error {
	if accumulator == nil || record.Table != accumulator.table || record.Ordinal != accumulator.count+1 {
		return ErrInvalidConfig
	}
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(record.Ordinal))
	_, _ = accumulator.hash.Write(ordinal[:])
	_, _ = accumulator.hash.Write(record.SourceKeyHMAC[:])
	_, _ = accumulator.hash.Write(record.PayloadHMAC[:])
	_, _ = accumulator.hash.Write(record.FieldHMAC[:])
	accumulator.count++
	return nil
}

func (accumulator *summaryAccumulator) Summary(runID string) TableSummary {
	var digest [sha256.Size]byte
	copy(digest[:], accumulator.hash.Sum(nil))
	return TableSummary{Table: accumulator.table, Count: accumulator.count, Digest: digest}
}

func EqualSummary(left, right Summary) bool {
	if left.RunID != right.RunID || len(left.Tables) != len(right.Tables) {
		return false
	}
	for index := range left.Tables {
		if left.Tables[index] != right.Tables[index] {
			return false
		}
	}
	return true
}

func RequireReconciled(source, target Summary) error {
	if !EqualSummary(source, target) {
		return ErrReconciliationFailed
	}
	return nil
}

func IsPayloadConflict(err error) bool { return errors.Is(err, ErrPayloadConflict) }

package manifest

import (
	"fmt"
	"sort"
	"strings"
)

// DispositionDocument maps every source table to exactly one terminal
// migration disposition. Source/target names are explicit, so renamed V2
// tables need no legacy-table allowlist in the tool.
type DispositionDocument struct {
	Version int                `json:"version"`
	Tables  []TableDisposition `json:"tables"`
}

type TableDisposition struct {
	Source      TableKey  `json:"source"`
	Disposition string    `json:"disposition"`
	Target      *TableKey `json:"target,omitempty"`
}

var terminalDispositions = map[string]struct{}{
	"imported": {}, "archived": {}, "quarantined": {}, "reset_runtime": {},
	"rebuild": {}, "manual_reentry": {}, "deferred": {}, "no_data": {},
}

type ReconciliationReport struct {
	SourceManifestDigest                 string                    `json:"source_manifest_digest"`
	TargetManifestDigest                 string                    `json:"target_manifest_digest,omitempty"`
	SourceTableCount                     int                       `json:"source_table_count"`
	SourceRowCount                       int64                     `json:"source_row_count"`
	TerminalDispositionTableCount        int                       `json:"terminal_disposition_table_count"`
	TerminalDispositionSourceRowCount    int64                     `json:"terminal_disposition_source_row_count"`
	SourceCountEqualsTerminalDisposition bool                      `json:"source_count_equals_terminal_disposition"`
	Unclassified                         []TableKey                `json:"unclassified"`
	UnknownSources                       []TableKey                `json:"unknown_sources"`
	DuplicateSources                     []TableKey                `json:"duplicate_sources"`
	ByDisposition                        []DispositionSummary      `json:"by_disposition"`
	Tables                               []TableReconciliationLine `json:"tables"`
}

type DispositionSummary struct {
	Disposition      string `json:"disposition"`
	SourceTableCount int    `json:"source_table_count"`
	SourceRowCount   int64  `json:"source_row_count"`
}

type TableReconciliationLine struct {
	Source         TableKey  `json:"source"`
	SourceRowCount int64     `json:"source_row_count"`
	Disposition    string    `json:"disposition"`
	Target         *TableKey `json:"target,omitempty"`
	TargetRowCount *int64    `json:"target_row_count,omitempty"`
	TargetPresent  bool      `json:"target_present"`
}

func Reconcile(source Manifest, target *Manifest, document DispositionDocument) (ReconciliationReport, error) {
	if err := source.Normalize(); err != nil {
		return ReconciliationReport{}, err
	}
	if target != nil {
		if err := target.Normalize(); err != nil {
			return ReconciliationReport{}, err
		}
	}
	if document.Version == 0 {
		document.Version = Version
	}
	if document.Version != Version {
		return ReconciliationReport{}, fmt.Errorf("unsupported disposition version %d", document.Version)
	}
	report := ReconciliationReport{SourceManifestDigest: source.Digest, SourceTableCount: len(source.Tables)}
	if target != nil {
		report.TargetManifestDigest = target.Digest
	}
	sourceTables := source.TableMap()
	targetTables := map[string]Table{}
	if target != nil {
		targetTables = target.TableMap()
	}
	entries := make(map[string]TableDisposition, len(document.Tables))
	for _, entry := range document.Tables {
		entry.Disposition = strings.TrimSpace(strings.ToLower(entry.Disposition))
		key := entry.Source.String()
		if entry.Source.Schema == "" || entry.Source.Table == "" || entry.Disposition == "" {
			return ReconciliationReport{}, fmt.Errorf("disposition entry is incomplete")
		}
		if _, terminal := terminalDispositions[entry.Disposition]; !terminal {
			return ReconciliationReport{}, fmt.Errorf("source %s has non-terminal disposition %q", key, entry.Disposition)
		}
		if _, duplicate := entries[key]; duplicate {
			report.DuplicateSources = append(report.DuplicateSources, entry.Source)
			continue
		}
		entries[key] = entry
	}
	summaries := map[string]*DispositionSummary{}
	for _, sourceTable := range source.Tables {
		report.SourceRowCount += sourceTable.RowCount
		entry, classified := entries[sourceTable.TableKey.String()]
		if !classified {
			report.Unclassified = append(report.Unclassified, sourceTable.TableKey)
			continue
		}
		report.TerminalDispositionTableCount++
		report.TerminalDispositionSourceRowCount += sourceTable.RowCount
		summary := summaries[entry.Disposition]
		if summary == nil {
			summary = &DispositionSummary{Disposition: entry.Disposition}
			summaries[entry.Disposition] = summary
		}
		summary.SourceTableCount++
		summary.SourceRowCount += sourceTable.RowCount
		line := TableReconciliationLine{Source: sourceTable.TableKey, SourceRowCount: sourceTable.RowCount, Disposition: entry.Disposition, Target: entry.Target}
		if entry.Target != nil {
			if targetTable, found := targetTables[entry.Target.String()]; found {
				line.TargetPresent = true
				count := targetTable.RowCount
				line.TargetRowCount = &count
			}
		}
		report.Tables = append(report.Tables, line)
	}
	for key, entry := range entries {
		if _, found := sourceTables[key]; !found {
			report.UnknownSources = append(report.UnknownSources, entry.Source)
		}
	}
	for _, summary := range summaries {
		report.ByDisposition = append(report.ByDisposition, *summary)
	}
	sortTableKeys(report.Unclassified)
	sortTableKeys(report.UnknownSources)
	sortTableKeys(report.DuplicateSources)
	sort.Slice(report.ByDisposition, func(left, right int) bool {
		return report.ByDisposition[left].Disposition < report.ByDisposition[right].Disposition
	})
	sort.Slice(report.Tables, func(left, right int) bool {
		return report.Tables[left].Source.String() < report.Tables[right].Source.String()
	})
	report.SourceCountEqualsTerminalDisposition = len(report.Unclassified) == 0 && len(report.UnknownSources) == 0 && len(report.DuplicateSources) == 0 && report.SourceTableCount == report.TerminalDispositionTableCount && report.SourceRowCount == report.TerminalDispositionSourceRowCount
	return report, nil
}

func sortTableKeys(keys []TableKey) {
	sort.Slice(keys, func(left, right int) bool { return keys[left].String() < keys[right].String() })
}

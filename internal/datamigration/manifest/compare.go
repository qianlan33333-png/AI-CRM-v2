package manifest

import "sort"

// Comparison reports only metadata and aggregate differences between two
// manifests. It does not assume that source and target table names are equal.
type Comparison struct {
	LeftDigest  string       `json:"left_digest"`
	RightDigest string       `json:"right_digest"`
	Added       []Table      `json:"added"`
	Removed     []Table      `json:"removed"`
	Changed     []TableDelta `json:"changed"`
	Equal       bool         `json:"equal"`
}

type TableDelta struct {
	Table             TableKey  `json:"table"`
	LeftRowCount      int64     `json:"left_row_count"`
	RightRowCount     int64     `json:"right_row_count"`
	LeftSchemaDigest  string    `json:"left_schema_digest"`
	RightSchemaDigest string    `json:"right_schema_digest"`
	LeftWatermark     Watermark `json:"left_watermark"`
	RightWatermark    Watermark `json:"right_watermark"`
}

func Compare(left, right Manifest) (Comparison, error) {
	if err := left.Normalize(); err != nil {
		return Comparison{}, err
	}
	if err := right.Normalize(); err != nil {
		return Comparison{}, err
	}
	comparison := Comparison{LeftDigest: left.Digest, RightDigest: right.Digest}
	leftTables, rightTables := left.TableMap(), right.TableMap()
	for key, leftTable := range leftTables {
		rightTable, found := rightTables[key]
		if !found {
			comparison.Removed = append(comparison.Removed, leftTable)
			continue
		}
		if leftTable.SchemaDigest != rightTable.SchemaDigest || leftTable.RowCount != rightTable.RowCount || leftTable.Watermark != rightTable.Watermark {
			comparison.Changed = append(comparison.Changed, TableDelta{
				Table: leftTable.TableKey, LeftRowCount: leftTable.RowCount, RightRowCount: rightTable.RowCount,
				LeftSchemaDigest: leftTable.SchemaDigest, RightSchemaDigest: rightTable.SchemaDigest,
				LeftWatermark: leftTable.Watermark, RightWatermark: rightTable.Watermark,
			})
		}
	}
	for key, rightTable := range rightTables {
		if _, found := leftTables[key]; !found {
			comparison.Added = append(comparison.Added, rightTable)
		}
	}
	sort.Slice(comparison.Added, func(left, right int) bool {
		return comparison.Added[left].TableKey.String() < comparison.Added[right].TableKey.String()
	})
	sort.Slice(comparison.Removed, func(left, right int) bool {
		return comparison.Removed[left].TableKey.String() < comparison.Removed[right].TableKey.String()
	})
	sort.Slice(comparison.Changed, func(left, right int) bool {
		return comparison.Changed[left].Table.String() < comparison.Changed[right].Table.String()
	})
	comparison.Equal = len(comparison.Added) == 0 && len(comparison.Removed) == 0 && len(comparison.Changed) == 0
	return comparison, nil
}

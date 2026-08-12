// Package sqlddl builds a deterministic model of the tables and table
// constraints that remain effective after applying top-level PostgreSQL DDL.
package sqlddl

import "sort"

// ConstraintKind is the canonical class of a table constraint.
type ConstraintKind string

const (
	ConstraintCheck      ConstraintKind = "check"
	ConstraintPrimaryKey ConstraintKind = "primary_key"
	ConstraintUnique     ConstraintKind = "unique"
	ConstraintForeignKey ConstraintKind = "foreign_key"
	ConstraintExclude    ConstraintKind = "exclude"
)

// Constraint is a canonical table constraint. Name is empty for an anonymous
// constraint declared directly in CREATE TABLE.
type Constraint struct {
	Name      string
	Kind      ConstraintKind
	Canonical string
}

// Column is a top-level CREATE TABLE column item. Canonical contains the
// normalized complete item, including its type and column modifiers.
type Column struct {
	Name      string
	Canonical string
}

// Table is the final effective model for one table.
type Table struct {
	Name                 string
	Columns              map[string]Column
	Constraints          map[string]Constraint
	AnonymousConstraints []Constraint
}

// ColumnNames returns deterministic canonical column names.
func (t Table) ColumnNames() []string {
	names := make([]string, 0, len(t.Columns))
	for name := range t.Columns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ConstraintNames returns deterministic names for named effective
// constraints. Anonymous constraints are available through
// AnonymousConstraints.
func (t Table) ConstraintNames() []string {
	names := make([]string, 0, len(t.Constraints))
	for name := range t.Constraints {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EffectiveConstraints returns all final constraints in canonical order.
func (t Table) EffectiveConstraints() []Constraint {
	constraints := make([]Constraint, 0, len(t.Constraints)+len(t.AnonymousConstraints))
	for _, constraint := range t.Constraints {
		constraints = append(constraints, constraint)
	}
	constraints = append(constraints, t.AnonymousConstraints...)
	sort.Slice(constraints, func(i, j int) bool {
		if constraints[i].Name != constraints[j].Name {
			return constraints[i].Name < constraints[j].Name
		}
		if constraints[i].Kind != constraints[j].Kind {
			return constraints[i].Kind < constraints[j].Kind
		}
		return constraints[i].Canonical < constraints[j].Canonical
	})
	return constraints
}

// Catalog is the final effective table catalog.
type Catalog struct {
	Tables map[string]*Table
}

// Table returns a value copy of a table by canonical qualified name.
func (c Catalog) Table(name string) (Table, bool) {
	table, ok := c.Tables[name]
	if !ok {
		return Table{}, false
	}
	return *table, true
}

// TableNames returns deterministic canonical table names.
func (c Catalog) TableNames() []string {
	names := make([]string, 0, len(c.Tables))
	for name := range c.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

package sqlddl

import (
	"fmt"
	"strings"
)

// Parse applies supported top-level PostgreSQL table DDL in source order and
// returns the final effective catalog. Non-table statements are ignored.
func Parse(source string) (Catalog, error) {
	tokens, err := lex(source)
	if err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{Tables: make(map[string]*Table)}
	for index, statement := range splitStatements(tokens) {
		if len(statement) == 0 {
			continue
		}
		if err := validateBalanced(statement); err != nil {
			return Catalog{}, fmt.Errorf("sqlddl: statement %d: %w", index+1, err)
		}
		if err := catalog.apply(statement); err != nil {
			return Catalog{}, fmt.Errorf("sqlddl: statement %d: %w", index+1, err)
		}
	}
	return catalog, nil
}

func validateBalanced(tokens []token) error {
	depth := 0
	for _, item := range tokens {
		if item.kind != tokenSymbol {
			continue
		}
		switch item.raw {
		case "(":
			depth++
		case ")":
			depth--
			if depth < 0 {
				return fmt.Errorf("unexpected closing parenthesis")
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unterminated parenthesis")
	}
	return nil
}

func splitStatements(tokens []token) [][]token {
	statements := make([][]token, 0)
	start := 0
	for index, item := range tokens {
		if item.kind == tokenSymbol && item.raw == ";" {
			statements = append(statements, tokens[start:index])
			start = index + 1
		}
	}
	if start < len(tokens) {
		statements = append(statements, tokens[start:])
	}
	return statements
}

func (c Catalog) apply(statement []token) error {
	switch {
	case keywords(statement, "create", "table"):
		return c.applyCreateTable(statement[2:])
	case keywords(statement, "alter", "table"):
		return c.applyAlterTable(statement[2:])
	case keywords(statement, "drop", "table"):
		return c.applyDropTable(statement[2:])
	default:
		return nil
	}
}

func (c Catalog) applyCreateTable(tokens []token) error {
	ifNotExists := false
	if keywords(tokens, "if", "not", "exists") {
		ifNotExists = true
		tokens = tokens[3:]
	}
	open := findTopLevelSymbol(tokens, "(")
	if open <= 0 {
		return fmt.Errorf("CREATE TABLE is missing its top-level body")
	}
	close, ok := matchingClose(tokens, open)
	if !ok {
		return fmt.Errorf("CREATE TABLE has an unterminated body")
	}
	name, err := qualifiedName(tokens[:open])
	if err != nil {
		return fmt.Errorf("CREATE TABLE name: %w", err)
	}
	if _, exists := c.Tables[name]; exists {
		if ifNotExists {
			return nil
		}
		return fmt.Errorf("table %s already exists", name)
	}
	table := &Table{
		Name:        name,
		Columns:     make(map[string]Column),
		Constraints: make(map[string]Constraint),
	}
	for _, item := range splitTopLevel(tokens[open+1:close], ",") {
		if len(item) == 0 {
			return fmt.Errorf("CREATE TABLE %s contains an empty item", name)
		}
		constraint, isConstraint, err := parseConstraint(item)
		if err != nil {
			return fmt.Errorf("CREATE TABLE %s: %w", name, err)
		}
		if isConstraint {
			if err := addConstraint(table, constraint); err != nil {
				return err
			}
			continue
		}
		columnName, ok := item[0].identifier()
		if !ok {
			return fmt.Errorf("CREATE TABLE %s has an invalid column item %q", name, canonical(item))
		}
		if _, exists := table.Columns[columnName]; exists {
			return fmt.Errorf("CREATE TABLE %s repeats column %s", name, columnName)
		}
		table.Columns[columnName] = Column{Name: columnName, Canonical: canonical(item)}
	}
	c.Tables[name] = table
	return nil
}

func (c Catalog) applyAlterTable(tokens []token) error {
	ifExists := false
	if keywords(tokens, "if", "exists") {
		ifExists = true
		tokens = tokens[2:]
	}
	if len(tokens) > 0 && tokens[0].keyword("only") {
		tokens = tokens[1:]
	}
	action := firstTopLevelKeyword(tokens, "add", "drop")
	if action <= 0 {
		return fmt.Errorf("ALTER TABLE has no supported top-level action")
	}
	name, err := qualifiedName(tokens[:action])
	if err != nil {
		return fmt.Errorf("ALTER TABLE name: %w", err)
	}
	table, exists := c.Tables[name]
	if !exists {
		if ifExists {
			return nil
		}
		return fmt.Errorf("ALTER TABLE references unknown table %s", name)
	}
	for _, item := range splitTopLevel(tokens[action:], ",") {
		if len(item) == 0 {
			return fmt.Errorf("ALTER TABLE %s contains an empty action", name)
		}
		switch {
		case item[0].keyword("add"):
			body := item[1:]
			constraint, isConstraint, err := parseConstraint(body)
			if err != nil {
				return fmt.Errorf("ALTER TABLE %s ADD: %w", name, err)
			}
			if !isConstraint {
				return fmt.Errorf("ALTER TABLE %s ADD supports constraints only", name)
			}
			if err := addConstraint(table, constraint); err != nil {
				return err
			}
		case item[0].keyword("drop"):
			if err := dropConstraint(table, item[1:]); err != nil {
				return fmt.Errorf("ALTER TABLE %s DROP: %w", name, err)
			}
		default:
			return fmt.Errorf("ALTER TABLE %s has unsupported action %q", name, canonical(item))
		}
	}
	return nil
}

func (c Catalog) applyDropTable(tokens []token) error {
	ifExists := false
	if keywords(tokens, "if", "exists") {
		ifExists = true
		tokens = tokens[2:]
	}
	if len(tokens) > 0 && tokens[0].keyword("only") {
		tokens = tokens[1:]
	}
	for len(tokens) > 0 && (tokens[len(tokens)-1].keyword("cascade") || tokens[len(tokens)-1].keyword("restrict")) {
		tokens = tokens[:len(tokens)-1]
	}
	name, err := qualifiedName(tokens)
	if err != nil {
		return fmt.Errorf("DROP TABLE name: %w", err)
	}
	if _, exists := c.Tables[name]; !exists && !ifExists {
		return fmt.Errorf("DROP TABLE references unknown table %s", name)
	}
	delete(c.Tables, name)
	return nil
}

func parseConstraint(tokens []token) (Constraint, bool, error) {
	if len(tokens) == 0 {
		return Constraint{}, false, nil
	}
	constraint := Constraint{}
	definition := tokens
	if tokens[0].keyword("constraint") {
		if len(tokens) < 3 {
			return Constraint{}, true, fmt.Errorf("CONSTRAINT is missing a name or definition")
		}
		name, ok := tokens[1].identifier()
		if !ok {
			return Constraint{}, true, fmt.Errorf("CONSTRAINT has invalid name %q", tokens[1].raw)
		}
		constraint.Name = name
		definition = tokens[2:]
	}
	kind, ok := constraintKind(definition)
	if !ok {
		if constraint.Name != "" {
			return Constraint{}, true, fmt.Errorf("CONSTRAINT %s has an unsupported definition", constraint.Name)
		}
		return Constraint{}, false, nil
	}
	constraint.Kind = kind
	constraint.Canonical = canonical(definition)
	return constraint, true, nil
}

func constraintKind(tokens []token) (ConstraintKind, bool) {
	switch {
	case keywords(tokens, "check"):
		return ConstraintCheck, true
	case keywords(tokens, "primary", "key"):
		return ConstraintPrimaryKey, true
	case keywords(tokens, "unique"):
		return ConstraintUnique, true
	case keywords(tokens, "foreign", "key"):
		return ConstraintForeignKey, true
	case keywords(tokens, "exclude"):
		return ConstraintExclude, true
	default:
		return "", false
	}
}

func addConstraint(table *Table, constraint Constraint) error {
	if constraint.Name == "" {
		table.AnonymousConstraints = append(table.AnonymousConstraints, constraint)
		return nil
	}
	if _, exists := table.Constraints[constraint.Name]; exists {
		return fmt.Errorf("table %s repeats constraint %s", table.Name, constraint.Name)
	}
	table.Constraints[constraint.Name] = constraint
	return nil
}

func dropConstraint(table *Table, tokens []token) error {
	if len(tokens) == 0 || !tokens[0].keyword("constraint") {
		return fmt.Errorf("only DROP CONSTRAINT is supported")
	}
	tokens = tokens[1:]
	ifExists := false
	if keywords(tokens, "if", "exists") {
		ifExists = true
		tokens = tokens[2:]
	}
	if len(tokens) == 0 {
		return fmt.Errorf("DROP CONSTRAINT is missing a name")
	}
	name, ok := tokens[0].identifier()
	if !ok {
		return fmt.Errorf("DROP CONSTRAINT has invalid name %q", tokens[0].raw)
	}
	if _, exists := table.Constraints[name]; !exists {
		if ifExists {
			return nil
		}
		return fmt.Errorf("unknown constraint %s", name)
	}
	delete(table.Constraints, name)
	return nil
}

func qualifiedName(tokens []token) (string, error) {
	if len(tokens) == 0 {
		return "", fmt.Errorf("missing identifier")
	}
	parts := make([]string, 0, 2)
	wantIdentifier := true
	for _, item := range tokens {
		if wantIdentifier {
			identifier, ok := item.identifier()
			if !ok {
				return "", fmt.Errorf("invalid identifier token %q", item.raw)
			}
			parts = append(parts, identifier)
			wantIdentifier = false
			continue
		}
		if item.kind != tokenSymbol || item.raw != "." {
			return "", fmt.Errorf("unexpected token %q", item.raw)
		}
		wantIdentifier = true
	}
	if wantIdentifier {
		return "", fmt.Errorf("identifier ends with a dot")
	}
	return strings.Join(parts, "."), nil
}

func keywords(tokens []token, words ...string) bool {
	if len(tokens) < len(words) {
		return false
	}
	for index, word := range words {
		if !tokens[index].keyword(word) {
			return false
		}
	}
	return true
}

func findTopLevelSymbol(tokens []token, symbol string) int {
	depth := 0
	for index, item := range tokens {
		if item.kind != tokenSymbol {
			continue
		}
		switch item.raw {
		case "(":
			if depth == 0 && symbol == "(" {
				return index
			}
			depth++
		case ")":
			depth--
		case symbol:
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func matchingClose(tokens []token, open int) (int, bool) {
	depth := 0
	for index := open; index < len(tokens); index++ {
		if tokens[index].kind != tokenSymbol {
			continue
		}
		switch tokens[index].raw {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return index, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func splitTopLevel(tokens []token, separator string) [][]token {
	var result [][]token
	start, depth := 0, 0
	for index, item := range tokens {
		if item.kind != tokenSymbol {
			continue
		}
		switch item.raw {
		case "(":
			depth++
		case ")":
			depth--
		case separator:
			if depth == 0 {
				result = append(result, tokens[start:index])
				start = index + 1
			}
		}
	}
	return append(result, tokens[start:])
}

func firstTopLevelKeyword(tokens []token, words ...string) int {
	depth := 0
	for index, item := range tokens {
		if item.kind == tokenSymbol {
			switch item.raw {
			case "(":
				depth++
			case ")":
				depth--
			}
			continue
		}
		if depth != 0 {
			continue
		}
		for _, word := range words {
			if item.keyword(word) {
				return index
			}
		}
	}
	return -1
}

func canonical(tokens []token) string {
	var builder strings.Builder
	for index, item := range tokens {
		raw := item.raw
		if item.kind == tokenWord {
			raw = strings.ToLower(raw)
		}
		if index > 0 && needsSpace(tokens[index-1], item) {
			builder.WriteByte(' ')
		}
		builder.WriteString(raw)
	}
	return builder.String()
}

func needsSpace(previous, current token) bool {
	if current.kind == tokenSymbol && (current.raw == ")" || current.raw == "," || current.raw == "." || current.raw == "::") {
		return false
	}
	if previous.kind == tokenSymbol && (previous.raw == "(" || previous.raw == "." || previous.raw == "::") {
		return false
	}
	return true
}

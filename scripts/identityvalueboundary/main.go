// identityvalueboundary verifies the R5-C1 identity-value persistence boundary.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	rawIdentity = regexp.MustCompile(`(?i)(^|[^a-z0-9])(raw_(identity|value|identifier|external_userid|unionid|openid|phone)|(identity|value|identifier|external_userid|unionid|openid|phone)_raw)([^a-z0-9]|$)`)
	normalized  = regexp.MustCompile(`(?i)\bnormalized_value\b`)
	fingerprint = regexp.MustCompile(`(?i)fingerprint`)
	writeTarget = regexp.MustCompile(`(?is)\b(create\s+table|alter\s+table|insert\s+into|update)\s+([a-z0-9_\."]+)`)
	fromTable   = regexp.MustCompile(`(?is)\b(from|join)\s+(public\.)?identities\b`)
	exactLookup = regexp.MustCompile(`(?is)\bwhere\b[^;]*\bkind\s*=\s*\$[0-9]+[^;]*\bscope\s*=\s*\$[0-9]+[^;]*\bnormalized_value\s*=\s*\$[0-9]+`)
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("unexpected arguments")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}
	if err := checkRepository(absRoot); err != nil {
		fatalf("%v", err)
	}
	fmt.Println("identity-value-boundary: PASS")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "identity-value-boundary: "+format+"\n", args...)
	os.Exit(1)
}

func checkRepository(root string) error {
	for _, tree := range []string{"cmd", "internal", "migrations"} {
		if err := walkRegularTree(root, tree, checkFile); err != nil {
			return err
		}
	}
	if err := checkResolveOpenAPI(root); err != nil {
		return err
	}
	return nil
}

func walkRegularTree(root, tree string, check func(string, string) error) error {
	treeRoot := filepath.Join(root, tree)
	info, err := os.Lstat(treeRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("regular directory required: %s", tree)
	}
	return filepath.WalkDir(treeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("symlink or special path forbidden: %s", rel)
		}
		if info.IsDir() {
			return nil
		}
		return check(path, rel)
	})
}

func checkFile(path, rel string) error {
	switch filepath.Ext(path) {
	case ".go":
		if strings.HasSuffix(rel, "_test.go") || strings.Contains(rel, "/generated/") {
			return nil
		}
		return checkGo(path, rel)
	case ".sql":
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return checkSQL(rel, string(data))
	}
	return nil
}

func checkGo(path, rel string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	var result error
	ast.Inspect(file, func(node ast.Node) bool {
		if result != nil {
			return false
		}
		switch item := node.(type) {
		case *ast.Ident:
			if rawIdentity.MatchString(item.Name) {
				result = fmt.Errorf("raw identity marker forbidden in production source: %s", rel)
			}
		case *ast.BasicLit:
			if item.Kind == token.STRING && rawIdentity.MatchString(item.Value) {
				result = fmt.Errorf("raw identity marker forbidden in production source: %s", rel)
			}
		case *ast.FuncDecl:
			if item.Name != nil && item.Name.Name == "Resolve" && containsFingerprint(item.Type) {
				result = fmt.Errorf("fingerprint forbidden in Resolve contract: %s", rel)
			}
		case *ast.TypeSpec:
			if strings.HasPrefix(item.Name.Name, "Resolve") && containsFingerprint(item.Type) {
				result = fmt.Errorf("fingerprint forbidden in Resolve contract: %s", rel)
			}
		case *ast.Field:
			if len(item.Names) == 1 && item.Names[0].Name == "Resolve" && containsFingerprint(item.Type) {
				result = fmt.Errorf("fingerprint forbidden in Resolve contract: %s", rel)
			}
		}
		return result == nil
	})
	return result
}

func containsFingerprint(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		identifier, ok := child.(*ast.Ident)
		if ok && fingerprint.MatchString(identifier.Name) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func checkSQL(rel, source string) error {
	if rawIdentity.MatchString(source) {
		return fmt.Errorf("raw identity marker forbidden in storage source: %s", rel)
	}
	for _, statement := range splitStatements(stripComments(source)) {
		if strings.HasPrefix(rel, "internal/identity/store/") && fingerprint.MatchString(statement) && strings.Contains(strings.ToLower(statement), "where") {
			return fmt.Errorf("fingerprint cannot be used as an identity Resolve surrogate: %s", rel)
		}
		if !normalized.MatchString(statement) {
			continue
		}
		if !normalizedUseAllowed(rel, statement) {
			return fmt.Errorf("normalized_value allowed only in identity-owned identities storage or exact lookup: %s", rel)
		}
	}
	return nil
}

func normalizedUseAllowed(rel, statement string) bool {
	if match := writeTarget.FindStringSubmatch(statement); len(match) == 3 {
		return tableName(match[2]) == "identities" && (rel == "migrations/00010_identity_storage.sql" || strings.HasPrefix(rel, "internal/identity/store/"))
	}
	return strings.HasPrefix(rel, "internal/identity/store/") && fromTable.MatchString(statement) && exactLookup.MatchString(statement)
}

func tableName(value string) string {
	parts := strings.Split(value, ".")
	return strings.ToLower(strings.Trim(parts[len(parts)-1], `"`))
}

func checkResolveOpenAPI(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "api", "openapi.yaml"))
	if err != nil {
		return fmt.Errorf("read api/openapi.yaml: %w", err)
	}
	if rawIdentity.Match(data) {
		return fmt.Errorf("raw identity marker forbidden in OpenAPI")
	}
	if normalized.Match(data) {
		return fmt.Errorf("normalized_value is storage-only and forbidden in OpenAPI")
	}
	lines := strings.Split(string(data), "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "ResolveIdentityRequest:" {
			start = index
			break
		}
	}
	if start < 0 {
		return fmt.Errorf("ResolveIdentityRequest schema missing")
	}
	indent := len(lines[start]) - len(strings.TrimLeft(lines[start], " "))
	for _, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(line)-len(strings.TrimLeft(line, " ")) <= indent {
			break
		}
		if rawIdentity.MatchString(trimmed) || fingerprint.MatchString(trimmed) {
			return fmt.Errorf("raw identity or fingerprint forbidden in ResolveIdentityRequest")
		}
	}
	return nil
}

func stripComments(source string) string {
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		if marker := strings.Index(line, "--"); marker >= 0 {
			lines[index] = line[:marker]
		}
	}
	return strings.Join(lines, "\n")
}

func splitStatements(source string) []string {
	var statements []string
	start := 0
	inString := false
	for index := 0; index < len(source); index++ {
		if source[index] == '\'' {
			if inString && index+1 < len(source) && source[index+1] == '\'' {
				index++
				continue
			}
			inString = !inString
		}
		if source[index] == ';' && !inString {
			statements = append(statements, source[start:index+1])
			start = index + 1
		}
	}
	if strings.TrimSpace(source[start:]) != "" {
		statements = append(statements, source[start:])
	}
	return statements
}

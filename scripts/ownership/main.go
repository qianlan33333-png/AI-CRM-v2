package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type policy struct {
	domains map[string]bool
	tables  map[string]string
}

var (
	ignoredSQL = []*regexp.Regexp{
		regexp.MustCompile(`(?s)'(?:''|[^'])*'`),
		regexp.MustCompile(`(?s)/[*].*?[*]/`),
		regexp.MustCompile(`(?m)--.*$`),
		regexp.MustCompile(`(?i)\bdo\s+update\s+set\b`),
	}
	writePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bcreate\s+(?:temporary\s+|temp\s+)?table\s+(?:if\s+not\s+exists\s+)?([a-z0-9_.$"]+)`),
		regexp.MustCompile(`(?i)\binsert\s+into\s+([a-z0-9_.$"]+)`),
		regexp.MustCompile(`(?i)\bupdate\s+(?:only\s+)?([a-z0-9_.$"]+)(?:\s+[*])?(?:\s+(?:as\s+)?[a-z0-9_$"]+)?\s+set\b`),
		regexp.MustCompile(`(?i)\bdelete\s+from\s+(?:only\s+)?([a-z0-9_.$"]+)`),
		regexp.MustCompile(`(?i)\bmerge\s+into\s+([a-z0-9_.$"]+)`),
		regexp.MustCompile(`(?i)\bcopy\s+([a-z0-9_.$"]+)(?:\s*\([^)]*\))?\s+from\b`),
	}
	truncatePattern = regexp.MustCompile(`(?i)\btruncate\s+(?:table\s+)?([^;]+)`)
)

func main() {
	root := flag.String("root", ".", "repository root")
	policyPath := flag.String("policy", "docs/architecture/table-ownership.yml", "ownership policy")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("unexpected arguments")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}
	p, err := loadPolicy(filepath.Join(absRoot, *policyPath))
	if err != nil {
		fatalf("policy: %v", err)
	}
	if err := walkInternal(absRoot, p); err != nil {
		fatalf("%v", err)
	}
	fmt.Println("ownership-lint: PASS")
}
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ownership-lint: "+format+"\n", args...)
	os.Exit(1)
}
func loadPolicy(path string) (*policy, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	p := &policy{domains: map[string]bool{}, tables: map[string]string{}}
	section, owner := "", ""
	blockTables := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		text := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 && strings.HasSuffix(text, ":") {
			section, owner, blockTables = strings.TrimSuffix(text, ":"), "", false
			continue
		}
		if section != "owners" && section != "external_owners" {
			continue
		}
		if indent == 2 && strings.HasSuffix(text, ":") {
			owner, blockTables = strings.TrimSuffix(text, ":"), false
			if section == "owners" {
				p.domains[owner] = false
			}
			continue
		}
		if owner == "" {
			continue
		}
		if indent == 4 && strings.HasPrefix(text, "package:") {
			if section == "owners" && strings.TrimSpace(strings.TrimPrefix(text, "package:")) != "internal/"+owner {
				return nil, fmt.Errorf("owner package mismatch: %s", owner)
			}
			p.domains[owner], blockTables = true, false
			continue
		}
		if indent == 4 && strings.HasPrefix(text, "tables:") {
			rest := strings.TrimSpace(strings.TrimPrefix(text, "tables:"))
			blockTables = rest == ""
			if rest != "" {
				for _, table := range strings.Split(strings.Trim(rest, "[]"), ",") {
					if err := addTable(p, section, owner, strings.TrimSpace(table)); err != nil {
						return nil, err
					}
				}
			}
			continue
		}
		if blockTables && indent == 6 && strings.HasPrefix(text, "- ") {
			if err := addTable(p, section, owner, strings.TrimSpace(strings.TrimPrefix(text, "- "))); err != nil {
				return nil, err
			}
		} else if indent <= 4 {
			blockTables = false
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for domain, packageSeen := range p.domains {
		if !packageSeen {
			return nil, fmt.Errorf("owner package missing: %s", domain)
		}
	}
	for table, want := range map[string]string{"customers": "contact", "customer_event_idempotency": "contact", "identities": "identity", "identity_operation_receipts": "identity", "outbound_batches": "outbound", "outbound_batch_chunks": "outbound", "outbound_tasks": "outbound", "outbound_enqueue_receipts": "outbound", "event_log": "events", "river_job": "external/river", "goose_db_version": "external/goose"} {
		if p.tables[table] != want {
			return nil, fmt.Errorf("critical ownership missing: %s", table)
		}
	}
	return p, nil
}
func addTable(p *policy, section, owner, table string) error {
	if table == "" {
		return nil
	}
	if section == "external_owners" {
		owner = "external/" + owner
	}
	if previous, exists := p.tables[table]; exists {
		return fmt.Errorf("duplicate table owner: %s (%s, %s)", table, previous, owner)
	}
	p.tables[table] = owner
	return nil
}
func walkInternal(root string, p *policy) error {
	for _, tree := range []string{"internal", "acceptance"} {
		if err := walkSQLTree(root, tree, p); err != nil {
			return err
		}
	}
	return nil
}
func walkSQLTree(root, tree string, p *policy) error {
	treeRoot := filepath.Join(root, tree)
	info, err := os.Lstat(treeRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("regular directory required: %s", tree)
	}
	return filepath.WalkDir(treeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("symlink or special path forbidden: %s", filepath.ToSlash(rel))
		}
		if info.IsDir() {
			return nil
		}
		source := sourceModule(filepath.ToSlash(rel))
		switch filepath.Ext(path) {
		case ".sql":
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return checkText(string(data), source, filepath.ToSlash(rel), p)
		case ".go":
			return checkGo(path, filepath.ToSlash(rel), source, p)
		}
		return nil
	})
}
func sourceModule(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) == 3 && parts[0] == "acceptance" && parts[1] == "contactfixture" {
		return "contact"
	}
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}
func checkGo(path, rel, source string, p *policy) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	for _, spec := range file.Imports {
		value, _ := strconv.Unquote(spec.Path.Value)
		lower := strings.ToLower(value)
		if !strings.HasPrefix(value, "github.com/qianlan33333-png/AI-CRM-v2/") && (strings.Contains(lower, "wecom") || strings.Contains(lower, "wechatwork") || strings.Contains(lower, "workwx")) && source != "wecom" && source != "outbound" {
			return fmt.Errorf("external WeCom client import forbidden in %s: %s", rel, value)
		}
	}
	var result error
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING || result != nil {
			return result == nil
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil {
			result = checkText(value, source, rel, p)
		}
		return result == nil
	})
	return result
}
func checkText(text, source, rel string, p *policy) error {
	cleaned := text
	for _, pattern := range ignoredSQL {
		cleaned = pattern.ReplaceAllString(cleaned, " ")
	}
	for _, pattern := range writePatterns {
		for _, match := range pattern.FindAllStringSubmatch(cleaned, -1) {
			if err := checkTable(match[1], source, rel, p); err != nil {
				return err
			}
		}
	}
	for _, match := range truncatePattern.FindAllStringSubmatch(cleaned, -1) {
		for _, target := range strings.Split(match[1], ",") {
			fields := strings.Fields(strings.TrimSpace(target))
			if len(fields) > 0 && strings.EqualFold(fields[0], "only") {
				fields = fields[1:]
			}
			if len(fields) > 0 {
				if err := checkTable(fields[0], source, rel, p); err != nil {
					return err
				}
			}
		}
	}
	return checkWeCom(text, source, rel)
}
func checkTable(raw, source, rel string, p *policy) error {
	parts := strings.Split(raw, ".")
	table := normalizeIdentifier(parts[len(parts)-1])
	schema := ""
	if len(parts) > 1 {
		schema = normalizeIdentifier(parts[len(parts)-2])
	}
	if schema == "acceptance_fixtures" {
		return nil
	}
	owner, exists := p.tables[table]
	if !exists {
		return fmt.Errorf("write to unknown table in %s: %s", rel, table)
	}
	if owner != source {
		return fmt.Errorf("table write ownership violation in %s: %s belongs to %s", rel, table, owner)
	}
	return nil
}
func normalizeIdentifier(value string) string {
	return strings.ToLower(strings.Trim(value, `"`))
}
func checkWeCom(text, source, rel string) error {
	lower := strings.ToLower(text)
	marker := strings.Index(lower, "/cgi-bin/")
	if marker < 0 {
		if lower == "https://qyapi.weixin.qq.com" && source == "wecom" {
			return nil
		}
		if strings.Contains(lower, "qyapi.weixin.qq.com") {
			return fmt.Errorf("unknown WeCom operation in %s", rel)
		}
		return nil
	}
	operation := lower[marker+len("/cgi-bin/"):]
	if cut := strings.IndexAny(operation, "?# \t\r\n\""); cut >= 0 {
		operation = operation[:cut]
	}
	write := map[string]bool{"message/send": true, "externalcontact/add_msg_template": true, "externalcontact/remind_groupmsg_send": true, "externalcontact/add_contact_way": true, "externalcontact/update_contact_way": true, "externalcontact/del_contact_way": true, "externalcontact/mark_tag": true}
	read := map[string]bool{"gettoken": true, "auth/getuserinfo": true, "externalcontact/get": true, "externalcontact/list": true, "externalcontact/batch/get_by_user": true, "externalcontact/get_follow_user_list": true, "externalcontact/groupchat/get": true, "externalcontact/groupchat/list": true, "externalcontact/get_corp_tag_list": true}
	if write[operation] && source == "outbound" || read[operation] && source == "wecom" {
		return nil
	}
	if !write[operation] && !read[operation] {
		return fmt.Errorf("unknown WeCom operation in %s: %s", rel, operation)
	}
	return fmt.Errorf("WeCom operation ownership violation in %s: %s", rel, operation)
}

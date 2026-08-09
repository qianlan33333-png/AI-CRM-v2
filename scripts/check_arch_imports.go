package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const internalPrefix = "github.com/qianlan33333-png/AI-CRM-v2/internal/"

var domains = map[string]bool{
	"contact": true, "identity": true, "segment": true, "automation": true,
	"outbound": true, "wecom": true, "ai": true, "survey": true,
	"gateway": true, "config": true, "events": true, "auth": true,
	"stats": true, "ops": true,
}

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
	for _, tree := range []string{"cmd", "internal"} {
		if err := checkTree(absRoot, tree); err != nil {
			fatalf("%v", err)
		}
	}
	fmt.Println("arch-import-lint: PASS")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "arch-import-lint: "+format+"\n", args...)
	os.Exit(1)
}

func checkTree(root, tree string) error {
	treeRoot := filepath.Join(root, tree)
	info, err := os.Lstat(treeRoot)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", tree, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("regular directory required: %s", tree)
	}
	return filepath.WalkDir(treeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect %s: %w", filepath.ToSlash(rel), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("symlink or special path forbidden: %s", filepath.ToSlash(rel))
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		return checkFile(path, filepath.ToSlash(rel))
	})
}

func checkFile(path, rel string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return fmt.Errorf("parse import in %s: %w", rel, err)
		}
		if err := checkImport(rel, importPath); err != nil {
			return err
		}
	}
	return nil
}

func checkImport(source, importPath string) error {
	if !strings.HasPrefix(importPath, internalPrefix) {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(importPath, internalPrefix), "/")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("invalid internal import in %s: %s", source, importPath)
	}
	destination := parts[0]
	if destination == "platform" || destination == "api" {
		return nil
	}
	if !domains[destination] {
		return fmt.Errorf("unknown internal module in %s: %s", source, importPath)
	}
	sourceDomain, composition := classifySource(source)
	if composition || sourceDomain == destination {
		return nil
	}
	if sourceDomain != "" && len(parts) == 2 && parts[1] == "port" {
		return nil
	}
	return fmt.Errorf("forbidden cross-module import in %s: %s", source, importPath)
}

func classifySource(source string) (string, bool) {
	parts := strings.Split(source, "/")
	if len(parts) >= 2 && parts[0] == "cmd" && parts[1] == "aicrm" {
		return "", true
	}
	if len(parts) >= 2 && parts[0] == "internal" && domains[parts[1]] {
		return parts[1], false
	}
	return "", false
}

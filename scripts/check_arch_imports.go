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
	"strconv"
	"strings"
)

const internalPrefix = "github.com/qianlan33333-png/AI-CRM-v2/internal/"

var domains = map[string]bool{
	"contact": true, "identity": true, "segment": true, "automation": true,
	"outbound": true, "wecom": true, "ai": true, "survey": true,
	"gateway": true, "config": true, "events": true, "auth": true,
	"stats": true, "product": true, "media": true, "coupon": true, "order": true, "ops": true, "adminops": true,
	"operationcycle": true, "pushcenter": true, "customer360": true,
	"hxc": true,
}

var compositionRoots = map[string]bool{
	"aicrm":               true,
	"aicrm-river-migrate": true,
	"aicrm-contact-perf":  true,
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
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
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
	if err := checkEnvironmentReads(file, rel); err != nil {
		return err
	}
	return checkRiverBoundary(file, rel)
}

func checkRiverBoundary(file *ast.File, source string) error {
	if strings.HasPrefix(source, "internal/platform/river/") || strings.HasPrefix(source, "internal/platform/jobqueue/") {
		return nil
	}
	schedulerBoundary := strings.HasPrefix(source, "internal/platform/scheduler/")
	riverPackages := make(map[string]bool)
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if strings.HasPrefix(importPath, "github.com/riverqueue/river/riverdriver") || importPath == "github.com/riverqueue/river/rivermigrate" {
			return fmt.Errorf("raw River driver forbidden in %s", source)
		}
		if importPath != "github.com/riverqueue/river" {
			continue
		}
		packageName := "river"
		if spec.Name != nil {
			packageName = spec.Name.Name
		}
		if packageName == "." {
			return fmt.Errorf("raw River dot import forbidden in %s", source)
		}
		if packageName != "_" {
			riverPackages[packageName] = true
		}
	}
	forbidden := map[string]bool{
		"Client": true, "NewClient": true, "QueueDefault": true,
		"NewWorkers": true, "AddWorker": true, "AddWorkerSafely": true,
	}
	periodicForbidden := map[string]bool{
		"NewPeriodicJob": true, "PeriodicJob": true, "PeriodicJobOpts": true,
		"PeriodicSchedule": true, "PeriodicInterval": true, "NeverSchedule": true,
	}
	var symbol string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, packageSelector := selector.X.(*ast.Ident)
		if packageSelector && riverPackages[identifier.Name] &&
			(forbidden[selector.Sel.Name] || (!schedulerBoundary && periodicForbidden[selector.Sel.Name])) {
			symbol = selector.Sel.Name
			return false
		}
		if !schedulerBoundary && selector.Sel.Name == "PeriodicJobs" {
			symbol = selector.Sel.Name
			return false
		}
		return true
	})
	if symbol != "" {
		return fmt.Errorf("raw or default River symbol forbidden in %s: %s", source, symbol)
	}
	return nil
}

func checkEnvironmentReads(file *ast.File, source string) error {
	if source == "internal/config/load.go" || strings.HasSuffix(source, "_test.go") {
		return nil
	}
	environmentPackages := make(map[string]bool)
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || (importPath != "os" && importPath != "syscall") {
			continue
		}
		packageName := importPath
		if spec.Name != nil {
			packageName = spec.Name.Name
		}
		if packageName == "." {
			return fmt.Errorf("scattered environment read forbidden in %s", source)
		}
		if packageName != "_" {
			environmentPackages[packageName] = true
		}
	}
	var forbidden bool
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok || !environmentPackages[identifier.Name] {
			return true
		}
		switch selector.Sel.Name {
		case "Getenv", "LookupEnv", "Environ", "ExpandEnv":
			forbidden = true
		}
		return true
	})
	if forbidden {
		return fmt.Errorf("scattered environment read forbidden in %s", source)
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
	if importPath == internalPrefix+"platform/scheduler" && source != "cmd/aicrm/scheduler.go" &&
		!strings.HasPrefix(source, "internal/platform/scheduler/") {
		return fmt.Errorf("scheduler registration import forbidden outside the unique catalog in %s", source)
	}
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
	if len(parts) >= 2 && parts[0] == "cmd" && compositionRoots[parts[1]] {
		return "", true
	}
	if len(parts) >= 2 && parts[0] == "internal" && domains[parts[1]] {
		return parts[1], false
	}
	return "", false
}

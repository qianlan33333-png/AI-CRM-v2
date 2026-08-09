package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const maxManifestBytes = 4 << 20

var (
	required = []string{"path", "methods", "route_name", "capability_owner", "runtime_owner", "layer", "external_effects", "data_source", "requires_auth", "rollback", "audience", "auth_scheme", "capability", "access_scope", "pii_level", "csrf", "rate_limit", "principal_types"}
	allowed  = map[string]bool{"path": true, "methods": true, "route_name": true, "capability_owner": true, "runtime_owner": true, "layer": true, "external_effects": true, "data_source": true, "requires_auth": true, "rollback": true, "audience": true, "auth_scheme": true, "capability": true, "access_scope": true, "pii_level": true, "csrf": true, "rate_limit": true, "principal_types": true, "client_purpose": true, "service_audience": true, "service_capability": true}
)

type export struct {
	SchemaVersion        int              `json:"schema_version"`
	SourceKind           string           `json:"source_kind"`
	SourceCommit         string           `json:"source_commit"`
	SourceManifest       string           `json:"source_manifest"`
	SourceManifestSHA256 string           `json:"source_manifest_sha256"`
	RouteCount           int              `json:"route_count"`
	Routes               []map[string]any `json:"routes"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "legacy-route-export:", err)
		os.Exit(2)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) != 2 || !validSHA(args[1]) {
		return fmt.Errorf("usage: legacy-route-export MANIFEST LOWERCASE_40_HEX_SOURCE_SHA")
	}
	info, err := os.Lstat(args[0])
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return fmt.Errorf("manifest must be a regular file no larger than %d bytes", maxManifestBytes)
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	routes, err := parse(data)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	payload := export{1, "legacy_origin_main", args[1], "docs/architecture/route_ownership_manifest.yml", hex.EncodeToString(digest[:]), len(routes), routes}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(payload)
}

func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func parse(data []byte) ([]map[string]any, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' || strings.ContainsAny(string(data), "\r\t\x00") {
		return nil, fmt.Errorf("manifest must be non-empty LF-only text ending in one newline")
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	if len(lines) < 2 || lines[0] != "routes:" {
		return nil, fmt.Errorf("manifest must start with routes:")
	}
	var routes []map[string]any
	var current map[string]any
	listKey := ""
	finish := func() error {
		if current == nil {
			return nil
		}
		for _, key := range required {
			if value, ok := current[key]; !ok || empty(value) {
				return fmt.Errorf("route %d missing non-empty field %s", len(routes)+1, key)
			}
		}
		for _, key := range []string{"methods", "principal_types"} {
			values, ok := current[key].([]string)
			if !ok {
				return fmt.Errorf("route %d field %s must be a list", len(routes)+1, key)
			}
			sort.Strings(values)
			for index := 1; index < len(values); index++ {
				if values[index] == values[index-1] {
					return fmt.Errorf("route %d field %s repeats %s", len(routes)+1, key, values[index])
				}
			}
		}
		routes = append(routes, current)
		return nil
	}
	for number, line := range lines[1:] {
		if line == "" {
			return nil, fmt.Errorf("line %d is blank", number+2)
		}
		if strings.HasPrefix(line, "- path: ") {
			if err := finish(); err != nil {
				return nil, err
			}
			current = map[string]any{"path": strings.TrimPrefix(line, "- path: ")}
			listKey = ""
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("line %d appears before the first route", number+2)
		}
		if strings.HasPrefix(line, "  - ") {
			item := strings.TrimPrefix(line, "  - ")
			values, ok := current[listKey].([]string)
			if !ok || item == "" || strings.TrimSpace(item) != item {
				return nil, fmt.Errorf("line %d has an invalid list item", number+2)
			}
			current[listKey] = append(values, item)
			continue
		}
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
			return nil, fmt.Errorf("line %d has unsupported YAML shape", number+2)
		}
		key, tail, ok := strings.Cut(line[2:], ":")
		if !ok || !allowed[key] || key == "path" {
			return nil, fmt.Errorf("line %d has an unknown or invalid field", number+2)
		}
		if _, duplicate := current[key]; duplicate {
			return nil, fmt.Errorf("line %d repeats field %s", number+2, key)
		}
		if tail == "" {
			if key != "methods" && key != "principal_types" {
				return nil, fmt.Errorf("line %d field %s cannot be a list", number+2, key)
			}
			current[key], listKey = []string{}, key
			continue
		}
		if !strings.HasPrefix(tail, " ") || strings.TrimSpace(tail[1:]) != tail[1:] || tail[1:] == "" {
			return nil, fmt.Errorf("line %d has an invalid scalar", number+2)
		}
		value := tail[1:]
		if key == "requires_auth" || key == "csrf" {
			if value != "true" && value != "false" {
				return nil, fmt.Errorf("line %d field %s must be boolean", number+2, key)
			}
			current[key] = value == "true"
		} else {
			current[key] = value
		}
		listKey = ""
	}
	if err := finish(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	sort.Slice(routes, func(i, j int) bool { return routeKey(routes[i]) < routeKey(routes[j]) })
	for _, route := range routes {
		key := routeKey(route)
		if seen[key] {
			return nil, fmt.Errorf("duplicate route %s", key)
		}
		seen[key] = true
	}
	return routes, nil
}

func empty(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == ""
	case []string:
		return len(typed) == 0
	}
	return false
}

func routeKey(route map[string]any) string {
	return route["path"].(string) + "\x00" + strings.Join(route["methods"].([]string), ",") + "\x00" + route["route_name"].(string)
}

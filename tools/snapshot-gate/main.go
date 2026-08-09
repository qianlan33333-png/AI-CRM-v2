// Command snapshot-gate validates generated behavior snapshots and compares
// ephemeral handler-test output with the frozen catalog.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const maxDocumentBytes = 4 << 20

type catalog struct {
	Version     int            `json:"version"`
	IgnorePaths []string       `json:"ignore_paths"`
	Cases       []snapshotCase `json:"cases"`
}

type snapshotCase struct {
	OperationID      string   `json:"operation_id"`
	CaseID           string   `json:"case_id"`
	Request          request  `json:"request"`
	ExpectedResponse response `json:"expected_response"`
}

type actualDocument struct {
	Version int          `json:"version"`
	Cases   []actualCase `json:"cases"`
}

type actualCase struct {
	OperationID    string   `json:"operation_id"`
	CaseID         string   `json:"case_id"`
	Request        request  `json:"request"`
	ActualResponse response `json:"actual_response"`
}

type request struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

type response struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "validate" && args[0] != "compare") {
		fmt.Fprintln(stderr, "usage: snapshot-gate validate|compare <catalog.json>")
		return 2
	}
	data, err := readRegularFile(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "snapshot-gate: %v\n", err)
		return 1
	}
	value, expectedBodies, err := validateCatalog(data)
	if err != nil {
		fmt.Fprintf(stderr, "snapshot-gate: %v\n", err)
		return 1
	}
	if args[0] == "validate" {
		fmt.Fprintf(stdout, "snapshot-gate: PASS (%d cases; validation only)\n", len(value.Cases))
		return 0
	}
	actualData, err := readLimited(stdin, "actual response")
	if err != nil {
		fmt.Fprintf(stderr, "snapshot-gate: %v\n", err)
		return 1
	}
	if err := compareCatalog(value, expectedBodies, actualData); err != nil {
		fmt.Fprintf(stderr, "snapshot-gate: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "snapshot-gate: PASS (%d cases compared)\n", len(value.Cases))
	return 0
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("catalog must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer file.Close()
	return readLimited(file, "catalog")
}

func readLimited(reader io.Reader, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, maxDocumentBytes)
	}
	return data, nil
}

func validateCatalog(data []byte) (catalog, map[string]any, error) {
	var value catalog
	if err := decodeStrict(data, &value); err != nil {
		return value, nil, fmt.Errorf("decode catalog: %w", err)
	}
	if value.Version != 1 {
		return value, nil, errors.New("version must be 1")
	}
	if value.IgnorePaths == nil {
		return value, nil, errors.New("ignore_paths must be an array")
	}
	if value.Cases == nil {
		return value, nil, errors.New("cases must be an array")
	}
	bodies := make(map[string]any, len(value.Cases))
	seen := make(map[string]bool, len(value.Cases))
	previous := ""
	for index := range value.Cases {
		item := &value.Cases[index]
		key, err := validateCase(item.OperationID, item.CaseID, item.Request, item.ExpectedResponse)
		if err != nil {
			return value, nil, fmt.Errorf("case %d: %w", index, err)
		}
		if seen[key] {
			return value, nil, fmt.Errorf("duplicate case %q", key)
		}
		if previous != "" && key <= previous {
			return value, nil, errors.New("cases must be sorted by operation_id and case_id")
		}
		seen[key], previous = true, key
		body, err := decodeJSONValue(item.ExpectedResponse.Body)
		if err != nil {
			return value, nil, fmt.Errorf("case %q expected response body: %w", key, err)
		}
		bodies[key] = body
	}
	if err := validateIgnorePaths(value.IgnorePaths, bodies, nil); err != nil {
		return value, nil, err
	}
	return value, bodies, nil
}

func validateCase(operationID, caseID string, req request, resp response) (string, error) {
	if err := validID(operationID, "operation_id"); err != nil {
		return "", err
	}
	if err := validID(caseID, "case_id"); err != nil {
		return "", err
	}
	if req.Method == "" || req.Method != strings.ToUpper(req.Method) || strings.IndexFunc(req.Method, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
		return "", errors.New("request method must contain uppercase ASCII letters")
	}
	if !strings.HasPrefix(req.Path, "/") || strings.ContainsAny(req.Path, "\r\n") {
		return "", errors.New("request path must start with / and contain no newline")
	}
	if len(req.Body) == 0 {
		return "", errors.New("request body is required; use null when absent")
	}
	if _, err := decodeJSONValue(req.Body); err != nil {
		return "", fmt.Errorf("request body: %w", err)
	}
	if resp.Status < 100 || resp.Status > 599 {
		return "", errors.New("response status must be between 100 and 599")
	}
	if len(resp.Body) == 0 {
		return "", errors.New("response body is required; use null when absent")
	}
	return operationID + "\x00" + caseID, nil
}

func validID(value, field string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 128 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s must be a nonblank identifier of at most 128 bytes", field)
	}
	return nil
}

func compareCatalog(expected catalog, expectedBodies map[string]any, data []byte) error {
	var actual actualDocument
	if err := decodeStrict(data, &actual); err != nil {
		return fmt.Errorf("decode actual response: %w", err)
	}
	if actual.Version != 1 || actual.Cases == nil {
		return errors.New("actual response must have version 1 and a cases array")
	}
	if len(actual.Cases) != len(expected.Cases) {
		return fmt.Errorf("actual response case count is %d; want %d", len(actual.Cases), len(expected.Cases))
	}
	actualBodies := make(map[string]any, len(actual.Cases))
	for index := range actual.Cases {
		want, got := expected.Cases[index], actual.Cases[index]
		key, err := validateCase(got.OperationID, got.CaseID, got.Request, got.ActualResponse)
		if err != nil {
			return fmt.Errorf("actual case %d: %w", index, err)
		}
		wantKey := want.OperationID + "\x00" + want.CaseID
		if key != wantKey {
			return fmt.Errorf("actual case %d key differs", index)
		}
		pointer := casePointer(want.OperationID, want.CaseID)
		if want.Request.Method != got.Request.Method {
			return fmt.Errorf("snapshot mismatch at %s/request/method", pointer)
		}
		if want.Request.Path != got.Request.Path {
			return fmt.Errorf("snapshot mismatch at %s/request/path", pointer)
		}
		wantRequestBody, _ := decodeJSONValue(want.Request.Body)
		gotRequestBody, _ := decodeJSONValue(got.Request.Body)
		if !reflect.DeepEqual(wantRequestBody, gotRequestBody) {
			return fmt.Errorf("snapshot mismatch at %s/request/body", pointer)
		}
		if want.ExpectedResponse.Status != got.ActualResponse.Status {
			return fmt.Errorf("snapshot mismatch at %s/response/status", pointer)
		}
		body, err := decodeJSONValue(got.ActualResponse.Body)
		if err != nil {
			return fmt.Errorf("actual case %q response body: %w", key, err)
		}
		actualBodies[key] = body
	}
	if err := validateIgnorePaths(expected.IgnorePaths, expectedBodies, actualBodies); err != nil {
		return err
	}
	ignored := make(map[string]bool, len(expected.IgnorePaths))
	for _, path := range expected.IgnorePaths {
		ignored[path] = true
	}
	for _, item := range expected.Cases {
		key := item.OperationID + "\x00" + item.CaseID
		pointer := casePointer(item.OperationID, item.CaseID) + "/response/body"
		if mismatch := firstMismatch(expectedBodies[key], actualBodies[key], pointer, ignored); mismatch != "" {
			return fmt.Errorf("snapshot mismatch at %s", mismatch)
		}
	}
	return nil
}

func validateIgnorePaths(paths []string, expected, actual map[string]any) error {
	seen := make(map[string]bool, len(paths))
	previous := ""
	for _, path := range paths {
		if seen[path] {
			return fmt.Errorf("duplicate ignore path %q", path)
		}
		if previous != "" && path <= previous {
			return errors.New("ignore_paths must be sorted")
		}
		seen[path], previous = true, path
		tokens, err := parsePointer(path)
		if err != nil || len(tokens) < 6 || tokens[0] != "cases" || tokens[3] != "response" || tokens[4] != "body" {
			return fmt.Errorf("invalid exact ignore path %q", path)
		}
		key := tokens[1] + "\x00" + tokens[2]
		want, ok := expected[key]
		if !ok {
			return fmt.Errorf("ignore path does not name a catalog case: %q", path)
		}
		value, ok := lookup(want, tokens[5:])
		if !ok || isContainer(value) {
			return fmt.Errorf("ignore path must name an existing scalar leaf: %q", path)
		}
		if actual != nil {
			value, ok = lookup(actual[key], tokens[5:])
			if !ok || isContainer(value) {
				return fmt.Errorf("ignore path did not match actual response: %q", path)
			}
		}
	}
	return nil
}

func firstMismatch(want, got any, pointer string, ignored map[string]bool) string {
	if ignored[pointer] {
		return ""
	}
	wantObject, wantIsObject := want.(map[string]any)
	gotObject, gotIsObject := got.(map[string]any)
	if wantIsObject || gotIsObject {
		if !wantIsObject || !gotIsObject {
			return pointer
		}
		keys := make([]string, 0, len(wantObject)+len(gotObject))
		seen := map[string]bool{}
		for key := range wantObject {
			keys = append(keys, key)
			seen[key] = true
		}
		for key := range gotObject {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			wantValue, wantOK := wantObject[key]
			gotValue, gotOK := gotObject[key]
			child := pointer + "/" + escapePointer(key)
			if !wantOK || !gotOK {
				if !ignored[child] {
					return child
				}
				continue
			}
			if mismatch := firstMismatch(wantValue, gotValue, child, ignored); mismatch != "" {
				return mismatch
			}
		}
		return ""
	}
	wantArray, wantIsArray := want.([]any)
	gotArray, gotIsArray := got.([]any)
	if wantIsArray || gotIsArray {
		if !wantIsArray || !gotIsArray || len(wantArray) != len(gotArray) {
			return pointer
		}
		for index := range wantArray {
			if mismatch := firstMismatch(wantArray[index], gotArray[index], pointer+"/"+strconv.Itoa(index), ignored); mismatch != "" {
				return mismatch
			}
		}
		return ""
	}
	if !reflect.DeepEqual(want, got) {
		return pointer
	}
	return ""
}

func decodeStrict(data []byte, target any) error {
	if err := rejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("document must contain exactly one JSON value")
	}
	return nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key must be a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate field %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("document must contain exactly one JSON value")
	}
	return nil
}

func decodeJSONValue(raw json.RawMessage) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("body must contain exactly one JSON value")
	}
	return value, nil
}

func casePointer(operationID, caseID string) string {
	return "/cases/" + escapePointer(operationID) + "/" + escapePointer(caseID)
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func parsePointer(value string) ([]string, error) {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "*") {
		return nil, errors.New("not an exact JSON Pointer")
	}
	parts := strings.Split(value[1:], "/")
	for index, part := range parts {
		var result strings.Builder
		for offset := 0; offset < len(part); offset++ {
			if part[offset] != '~' {
				result.WriteByte(part[offset])
				continue
			}
			if offset+1 >= len(part) || (part[offset+1] != '0' && part[offset+1] != '1') {
				return nil, errors.New("invalid JSON Pointer escape")
			}
			offset++
			if part[offset] == '0' {
				result.WriteByte('~')
			} else {
				result.WriteByte('/')
			}
		}
		parts[index] = result.String()
	}
	return parts, nil
}

func lookup(value any, tokens []string) (any, bool) {
	for _, token := range tokens {
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[token]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != token {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func isContainer(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

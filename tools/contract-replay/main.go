// Command contract-replay validates replay manifests before any request is sent.
// P0 intentionally supports only an empty manifest; execution arrives in P5.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const maxManifestBytes = 1 << 20

type manifest struct {
	Version int               `json:"version"`
	Cases   []json.RawMessage `json:"cases"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: contract-replay <manifest.json>")
		return 2
	}
	info, err := os.Lstat(args[0])
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(stderr, "contract-replay: manifest must be a regular file")
		return 1
	}
	file, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "contract-replay: open manifest: %v\n", err)
		return 1
	}
	defer file.Close()
	if err := validateManifest(file); err != nil {
		fmt.Fprintf(stderr, "contract-replay: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "contract-replay: PASS (0 cases; validation only)")
	return 0
}

func validateManifest(reader io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if len(data) > maxManifestBytes {
		return fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	value, err := decodeManifest(decoder)
	if err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("manifest must contain exactly one JSON value")
	}
	if value.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if value.Cases == nil {
		return fmt.Errorf("cases must be an array")
	}
	if len(value.Cases) != 0 {
		return fmt.Errorf("execution adapter is not implemented; cases must be empty")
	}
	return nil
}

func decodeManifest(decoder *json.Decoder) (manifest, error) {
	var value manifest
	token, err := decoder.Token()
	if err != nil {
		return value, fmt.Errorf("decode manifest: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return value, fmt.Errorf("decode manifest: top-level value must be an object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return value, fmt.Errorf("decode manifest: %w", err)
		}
		key, ok := token.(string)
		if !ok {
			return value, fmt.Errorf("decode manifest: object key must be a string")
		}
		if seen[key] {
			return value, fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		switch key {
		case "version":
			err = decoder.Decode(&value.Version)
		case "cases":
			err = decoder.Decode(&value.Cases)
		default:
			return value, fmt.Errorf("unknown field %q", key)
		}
		if err != nil {
			return value, fmt.Errorf("decode manifest field %q: %w", key, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return value, fmt.Errorf("decode manifest: %w", err)
	}
	return value, nil
}

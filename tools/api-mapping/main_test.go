package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func frozen(t *testing.T) ([]byte, routeDocument) {
	mapping, _ := os.ReadFile("../../docs/api-mapping.jsonl")
	routeData, _ := os.ReadFile("../../docs/evidence/p1/legacy-routes-6cb989c.json")
	var routes routeDocument
	_ = json.Unmarshal(routeData, &routes)
	return mapping, routes
}

func TestRejectsUnsafeMutations(t *testing.T) {
	mapping, routes := frozen(t)
	if _, _, err := validate(bytes.NewReader(mapping), routes, 781); err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(mapping, []byte(`"disposition":"UNREVIEWED"`), []byte(`"disposition":"MIGRATE"`), 1)
	if _, _, err := validate(bytes.NewReader(changed), routes, 781); bytes.Equal(changed, mapping) || err == nil {
		t.Fatal("mutation anchor missing or accepted")
	}
}

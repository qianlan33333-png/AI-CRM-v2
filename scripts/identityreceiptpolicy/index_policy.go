package main

import (
	"fmt"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/scripts/sqlddl"
)

// CheckMigration enforces only the final receipt-index policy. Other receipt
// and pending-event policies are deliberately outside this slice.
func CheckMigration(source string) error {
	up, err := gooseUp(source)
	if err != nil {
		return err
	}
	catalog, err := sqlddl.Parse(up)
	if err != nil {
		return fmt.Errorf("identity receipt index policy: parse migration: %w", err)
	}
	for _, name := range catalog.IndexNames() {
		index, _ := catalog.Index(name)
		if !isReceiptTable(index.Table) {
			continue
		}
		if index.Method == "gin" {
			return fmt.Errorf("identity receipt index policy: final receipt GIN index is forbidden: %s", index.Name)
		}
		if len(index.Keys) == 1 && index.Keys[0].Column == "state" {
			return fmt.Errorf("identity receipt index policy: final receipt state-only index is forbidden: %s", index.Name)
		}
	}
	return nil
}

func gooseUp(source string) (string, error) {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"
	if strings.Count(source, upMarker) != 1 || strings.Count(source, downMarker) != 1 {
		return "", fmt.Errorf("identity receipt index policy: migration must contain one goose Up and Down marker")
	}
	up := strings.Index(source, upMarker)
	down := strings.Index(source, downMarker)
	if down <= up {
		return "", fmt.Errorf("identity receipt index policy: goose markers are out of order")
	}
	return source[up+len(upMarker) : down], nil
}

func isReceiptTable(name string) bool {
	switch name {
	case "identity_operation_receipts",
		`"identity_operation_receipts"`,
		"public.identity_operation_receipts",
		`public."identity_operation_receipts"`,
		`"public".identity_operation_receipts`,
		`"public"."identity_operation_receipts"`:
		return true
	default:
		return false
	}
}

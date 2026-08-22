package main

import (
	"fmt"
	"os"

	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

func main() {
	databaseURL, present := os.LookupEnv("MIGRATION_TEST_DATABASE_URL")
	databaseName := os.Getenv("MIGRATION_TEST_DATABASE_NAME")
	if databaseName == "" {
		databaseName = "aicrm_test"
	}
	if !present || fixtures.ValidateDatabaseURLForDatabase(databaseURL, databaseName) != nil {
		fmt.Fprintln(os.Stderr, "MIGRATION_TEST_DATABASE_URL failed safe acceptance validation")
		os.Exit(2)
	}
}

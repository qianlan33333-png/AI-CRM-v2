package main

import (
	"fmt"
	"os"

	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

func main() {
	databaseURL, present := os.LookupEnv("MIGRATION_TEST_DATABASE_URL")
	if !present || fixtures.ValidateDatabaseURL(databaseURL) != nil {
		fmt.Fprintln(os.Stderr, "MIGRATION_TEST_DATABASE_URL failed safe acceptance validation")
		os.Exit(2)
	}
}

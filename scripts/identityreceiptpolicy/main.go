package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	migration := flag.String("migration", "migrations/00010_identity_storage.sql", "identity storage migration")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("unexpected arguments")
	}
	source, err := os.ReadFile(*migration)
	if err != nil {
		fatalf("read migration: %v", err)
	}
	if err := CheckMigration(string(source)); err != nil {
		fatalf("%v", err)
	}
	fmt.Println("identity-receipt-policy: PASS")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "identity-receipt-policy: "+format+"\n", args...)
	os.Exit(1)
}

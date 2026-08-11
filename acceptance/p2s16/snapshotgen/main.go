package main

import (
	"fmt"
	"os"

	p2s16 "github.com/qianlan33333-png/AI-CRM-v2/acceptance/p2s16"
)

func main() {
	document, err := p2s16.GenerateStageSnapshot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "p2-s16 snapshot generation failed")
		os.Exit(1)
	}
	if _, err = os.Stdout.Write(append(document, '\n')); err != nil {
		fmt.Fprintln(os.Stderr, "p2-s16 snapshot output failed")
		os.Exit(1)
	}
}

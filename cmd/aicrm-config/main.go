package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/deployment"
)

const usageLine = "Usage: aicrm-config --tier=<s|m|l> --output-dir=<directory>"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintln(stdout, usageLine)
		return 0
	}
	tierValue, outputDirectory, ok := parseArguments(args)
	if !ok {
		fmt.Fprintln(stderr, "aicrm-config: invalid arguments")
		fmt.Fprintln(stderr, usageLine)
		return 2
	}
	config, err := deployment.Lookup(tierValue)
	if err != nil {
		fmt.Fprintln(stderr, "aicrm-config: --tier must be one of s, m, l")
		fmt.Fprintln(stderr, usageLine)
		return 2
	}
	if err := deployment.Generate(config, outputDirectory); err != nil {
		message := "aicrm-config: configuration generation failed"
		if errors.Is(err, deployment.ErrUnsafeOutputPath) {
			message = "aicrm-config: output directory is unsafe"
		}
		fmt.Fprintln(stderr, message)
		return 1
	}
	fmt.Fprintf(stdout, "aicrm-config: generated tier %s configuration\n", config.Tier)
	return 0
}

func parseArguments(args []string) (tier, outputDirectory string, ok bool) {
	if len(args) != 2 {
		return "", "", false
	}
	for _, argument := range args {
		switch {
		case strings.HasPrefix(argument, "--tier=") && tier == "":
			tier = strings.TrimPrefix(argument, "--tier=")
		case strings.HasPrefix(argument, "--output-dir=") && outputDirectory == "":
			outputDirectory = strings.TrimPrefix(argument, "--output-dir=")
		default:
			return "", "", false
		}
	}
	return tier, outputDirectory, tier != "" && outputDirectory != ""
}

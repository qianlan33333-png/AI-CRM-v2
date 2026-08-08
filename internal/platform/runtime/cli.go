package runtime

import (
	"errors"
	"strings"
)

var errInvalidArguments = errors.New("invalid arguments")

func ParseCLI(args []string) (CLIResult, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return CLIResult{Help: true}, nil
	}
	var value string
	switch {
	case len(args) == 0 || (len(args) == 1 && args[0] == "--role"):
		return CLIResult{}, ErrInvalidRole
	case len(args) == 1 && strings.HasPrefix(args[0], "--role="):
		value = strings.TrimPrefix(args[0], "--role=")
	case len(args) == 2 && args[0] == "--role" && !strings.HasPrefix(args[1], "-"):
		value = args[1]
	default:
		return CLIResult{}, errInvalidArguments
	}
	role := Role(value)
	switch role {
	case RoleAPI, RoleWorker, RoleAll:
		return CLIResult{Role: role}, nil
	default:
		return CLIResult{}, ErrInvalidRole
	}
}

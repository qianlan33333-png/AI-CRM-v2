// Package config owns typed startup configuration and its validation boundary.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

const (
	databaseURLEnv        = "AICRM_DATABASE_URL"
	apiListenAddressEnv   = "AICRM_HTTP_LISTEN_ADDRESS"
	apiPoolMaxConnsEnv    = "AICRM_API_PGX_MAX_CONNS"
	workerPoolMaxConnsEnv = "AICRM_WORKER_PGX_MAX_CONNS"
)

var ErrInvalid = errors.New("invalid startup configuration")

// DatabaseURL is opaque so generic formatting cannot expose embedded credentials.
type DatabaseURL struct {
	value string
}

func (databaseURL DatabaseURL) Value() string { return databaseURL.value }
func (DatabaseURL) String() string            { return "[REDACTED]" }
func (DatabaseURL) GoString() string          { return "[REDACTED]" }

type Database struct {
	URL DatabaseURL
}

type API struct {
	ListenAddress string
	PoolMaxConns  int32
}

type Worker struct {
	PoolMaxConns int32
}

// Root contains only startup infrastructure settings. Persisted business settings
// and credentials are deliberately outside this slice.
type Root struct {
	Database Database
	API      API
	Worker   Worker
}

type validationError struct {
	problems []string
}

func (validation validationError) Error() string {
	return ErrInvalid.Error() + ": " + strings.Join(validation.problems, "; ")
}

func (validationError) Unwrap() error { return ErrInvalid }

type environmentLookup func(string) (string, bool)

func load(role appruntime.Role, lookup environmentLookup) (Root, error) {
	var root Root
	var problems []string

	databaseValue, databasePresent := lookup(databaseURLEnv)
	if !databasePresent || databaseValue == "" {
		problems = append(problems, "database.url is required")
	} else if !validDatabaseURL(databaseValue) {
		problems = append(problems, "database.url must be a valid postgres URL")
	} else {
		root.Database.URL = DatabaseURL{value: databaseValue}
	}

	needAPI, needWorker, roleValid := selectedComponents(role)
	if !roleValid {
		problems = append(problems, "process.role is invalid")
	}
	if needAPI {
		listenAddress, present := lookup(apiListenAddressEnv)
		if !present || listenAddress == "" {
			problems = append(problems, "api.listen_address is required")
		} else if !validListenAddress(listenAddress) {
			problems = append(problems, "api.listen_address must be host:port")
		} else {
			root.API.ListenAddress = listenAddress
		}
		root.API.PoolMaxConns = parsePositiveInt32(lookup, apiPoolMaxConnsEnv, "api.pool_max_conns", &problems)
	}
	if needWorker {
		root.Worker.PoolMaxConns = parsePositiveInt32(lookup, workerPoolMaxConnsEnv, "worker.pool_max_conns", &problems)
	}
	if len(problems) != 0 {
		return Root{}, validationError{problems: problems}
	}
	return root, nil
}

func selectedComponents(role appruntime.Role) (needAPI, needWorker, valid bool) {
	switch role {
	case appruntime.RoleAPI:
		return true, false, true
	case appruntime.RoleWorker:
		return false, true, true
	case appruntime.RoleAll:
		return true, true, true
	default:
		return false, false, false
	}
}

func validDatabaseURL(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return false
	}
	return parsed.Hostname() != "" && parsed.Path != "" && parsed.Path != "/" && parsed.Fragment == "" && parsed.Opaque == ""
}

func validListenAddress(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	_, portText, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portText)
	return err == nil && port >= 1 && port <= 65535
}

func parsePositiveInt32(lookup environmentLookup, environmentKey, field string, problems *[]string) int32 {
	value, present := lookup(environmentKey)
	if !present || value == "" {
		*problems = append(*problems, fmt.Sprintf("%s is required", field))
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		*problems = append(*problems, fmt.Sprintf("%s must be a positive integer", field))
		return 0
	}
	return int32(parsed)
}

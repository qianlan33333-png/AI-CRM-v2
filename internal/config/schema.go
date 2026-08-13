// Package config owns typed startup configuration and its validation boundary.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

const (
	databaseURLEnv         = "AICRM_DATABASE_URL"
	apiListenAddressEnv    = "AICRM_HTTP_LISTEN_ADDRESS"
	apiPoolMaxConnsEnv     = "AICRM_API_PGX_MAX_CONNS"
	workerPoolMaxConnsEnv  = "AICRM_WORKER_PGX_MAX_CONNS"
	criticalWorkersEnv     = "AICRM_RIVER_CRITICAL_MAX_WORKERS"
	eventWorkersEnv        = "AICRM_RIVER_EVENT_MAX_WORKERS"
	outboundWorkersEnv     = "AICRM_RIVER_OUTBOUND_MAX_WORKERS"
	syncWorkersEnv         = "AICRM_RIVER_SYNC_MAX_WORKERS"
	heavyWorkersEnv        = "AICRM_RIVER_HEAVY_MAX_WORKERS"
	aiWorkersEnv           = "AICRM_RIVER_AI_MAX_WORKERS"
	weComCallbackCorpIDEnv = "AICRM_WECOM_CALLBACK_CORP_ID"
	weComCallbackTokenEnv  = "AICRM_WECOM_CALLBACK_TOKEN"
	weComCallbackAESKeyEnv = "AICRM_WECOM_CALLBACK_AES_KEY"
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

// CallbackSecret is opaque to keep callback credentials out of generic logs
// and startup error messages.
type CallbackSecret struct{ value string }

func (secret CallbackSecret) Value() string { return secret.value }
func (CallbackSecret) String() string       { return "[REDACTED]" }
func (CallbackSecret) GoString() string     { return "[REDACTED]" }

// WeComCallback is either fully configured or disabled. Partial callback
// configuration is rejected at process startup rather than accepting traffic
// with an ambiguous security boundary.
type WeComCallback struct {
	Enabled        bool
	CorpID         string
	Token          CallbackSecret
	EncodingAESKey CallbackSecret
}

type WeCom struct {
	Callback WeComCallback
}

type Worker struct {
	PoolMaxConns int32
	Queues       QueueConcurrency
}

type QueueConcurrency struct {
	Critical int32
	Event    int32
	Outbound int32
	Sync     int32
	Heavy    int32
	AI       int32
}

func (queues QueueConcurrency) Total() int32 {
	return queues.Critical + queues.Event + queues.Outbound + queues.Sync + queues.Heavy + queues.AI
}

func (queues QueueConcurrency) valid() bool {
	return queues.Critical > 0 && queues.Event > 0 && queues.Outbound > 0 &&
		queues.Sync > 0 && queues.Heavy > 0 && queues.AI > 0
}

// Root contains only startup infrastructure settings. Persisted business settings
// and credentials are deliberately outside this slice.
type Root struct {
	Database Database
	API      API
	Worker   Worker
	WeCom    WeCom
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
		root.WeCom.Callback = parseWeComCallback(lookup, &problems)
	}
	if needWorker {
		root.Worker.PoolMaxConns = parsePositiveInt32(lookup, workerPoolMaxConnsEnv, "worker.pool_max_conns", &problems)
		root.Worker.Queues = QueueConcurrency{
			Critical: parsePositiveInt32(lookup, criticalWorkersEnv, "worker.queues.critical", &problems),
			Event:    parsePositiveInt32(lookup, eventWorkersEnv, "worker.queues.event", &problems),
			Outbound: parsePositiveInt32(lookup, outboundWorkersEnv, "worker.queues.outbound", &problems),
			Sync:     parsePositiveInt32(lookup, syncWorkersEnv, "worker.queues.sync", &problems),
			Heavy:    parsePositiveInt32(lookup, heavyWorkersEnv, "worker.queues.heavy", &problems),
			AI:       parsePositiveInt32(lookup, aiWorkersEnv, "worker.queues.ai", &problems),
		}
		queueTotal := root.Worker.Queues.Total()
		if root.Worker.PoolMaxConns > 0 && root.Worker.Queues.valid() && root.Worker.PoolMaxConns < queueTotal+2 {
			problems = append(problems, "worker.pool_max_conns must be at least queue concurrency total + 2")
		}
	}
	if len(problems) != 0 {
		return Root{}, validationError{problems: problems}
	}
	return root, nil
}

func parseWeComCallback(lookup environmentLookup, problems *[]string) WeComCallback {
	corpID, corpIDPresent := lookup(weComCallbackCorpIDEnv)
	token, tokenPresent := lookup(weComCallbackTokenEnv)
	aesKey, aesKeyPresent := lookup(weComCallbackAESKeyEnv)
	if !corpIDPresent && !tokenPresent && !aesKeyPresent {
		return WeComCallback{}
	}
	if !corpIDPresent || !tokenPresent || !aesKeyPresent || corpID == "" || token == "" || aesKey == "" {
		*problems = append(*problems, "wecom.callback requires corp_id, token, and aes_key together")
		return WeComCallback{}
	}
	if !validWeComCorpID(corpID) {
		*problems = append(*problems, "wecom.callback.corp_id is invalid")
	}
	if !validCallbackToken(token) {
		*problems = append(*problems, "wecom.callback.token is invalid")
	}
	if !validEncodingAESKey(aesKey) {
		*problems = append(*problems, "wecom.callback.aes_key is invalid")
	}
	return WeComCallback{Enabled: true, CorpID: corpID, Token: CallbackSecret{value: token}, EncodingAESKey: CallbackSecret{value: aesKey}}
}

func validWeComCorpID(value string) bool {
	if len(value) == 0 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func validCallbackToken(value string) bool {
	return len(value) > 0 && len(value) <= 256 && strings.TrimSpace(value) == value
}

func validEncodingAESKey(value string) bool {
	if len(value) != 43 || strings.TrimSpace(value) != value {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(value + "=")
	return err == nil && len(decoded) == 32
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
	if parsed.Hostname() == "" || parsed.Path == "" || parsed.Path == "/" || parsed.Fragment != "" || parsed.Opaque != "" {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	if capacities, present := query["description_cache_capacity"]; present {
		if len(capacities) != 1 {
			return false
		}
		capacity, err := strconv.Atoi(capacities[0])
		if err != nil || capacity < 1 {
			return false
		}
	}
	return true
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

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
	databaseURLEnv                    = "AICRM_DATABASE_URL"
	apiListenAddressEnv               = "AICRM_HTTP_LISTEN_ADDRESS"
	apiPoolMaxConnsEnv                = "AICRM_API_PGX_MAX_CONNS"
	workerPoolMaxConnsEnv             = "AICRM_WORKER_PGX_MAX_CONNS"
	criticalWorkersEnv                = "AICRM_RIVER_CRITICAL_MAX_WORKERS"
	eventWorkersEnv                   = "AICRM_RIVER_EVENT_MAX_WORKERS"
	outboundWorkersEnv                = "AICRM_RIVER_OUTBOUND_MAX_WORKERS"
	syncWorkersEnv                    = "AICRM_RIVER_SYNC_MAX_WORKERS"
	heavyWorkersEnv                   = "AICRM_RIVER_HEAVY_MAX_WORKERS"
	aiWorkersEnv                      = "AICRM_RIVER_AI_MAX_WORKERS"
	weComCallbackCorpIDEnv            = "AICRM_WECOM_CALLBACK_CORP_ID"
	weComCallbackTokenEnv             = "AICRM_WECOM_CALLBACK_TOKEN"
	weComCallbackAESKeyEnv            = "AICRM_WECOM_CALLBACK_AES_KEY"
	weComOAuthCorpIDEnv               = "AICRM_WECOM_OAUTH_CORP_ID"
	weComOAuthSecretEnv               = "AICRM_WECOM_OAUTH_SECRET"
	weComOAuthCallbackEnv             = "AICRM_WECOM_OAUTH_CALLBACK_URL"
	weComDirectorySyncEnabledEnv      = "AICRM_WECOM_DIRECTORY_SYNC_ENABLED"
	weComDirectorySyncStaffUserIDsEnv = "AICRM_WECOM_DIRECTORY_SYNC_STAFF_USER_IDS"
	weComSidebarCorpIDEnv             = "AICRM_WECOM_SIDEBAR_CORP_ID"
	weComSidebarSecretEnv             = "AICRM_WECOM_SIDEBAR_SECRET"
	weComSidebarCallbackEnv           = "AICRM_WECOM_SIDEBAR_CALLBACK_URL"
	weComSidebarAgentIDEnv            = "AICRM_WECOM_SIDEBAR_AGENT_ID"
	weComSidebarHostsEnv              = "AICRM_WECOM_SIDEBAR_ALLOWED_HOSTS"
	identityHMACKeyEnv                = "AICRM_IDENTITY_HMAC_KEY"
	apiClientJWTSecretEnv             = "AICRM_API_CLIENT_JWT_SECRET"
	surveyPublicKeyEnv                = "AICRM_SURVEY_PUBLIC_TOKEN_KEY"
	domainVerificationDirEnv          = "AICRM_DOMAIN_VERIFICATION_DIR"
	applicationEnvironmentEnv         = "AICRM_ENV"
	releaseSHAEnv                     = "AICRM_RELEASE_SHA"

	legacySecretKeyEnv                        = "SECRET_KEY"
	legacyWeChatShopCallbackTokenEnv          = "WECHAT_SHOP_CALLBACK_TOKEN"
	legacyAllowMissingWeChatShopCallbackToken = "AICRM_ALLOW_MISSING_WECHAT_SHOP_CALLBACK_TOKEN"
)

// legacyProductionEnvironmentEnvs are the legacy production aliases. The v2
// AICRM_ENV name is deliberately not one of them.
var legacyProductionEnvironmentEnvs = []string{"AICRM_NEXT_ENV", "ENVIRONMENT", "APP_ENV", "FLASK_ENV"}

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

// DomainVerification is optional startup configuration for the public,
// filesystem-backed verification reader. The reader owns path safety checks.
type DomainVerification struct {
	Directory string
}

// Release contains non-secret deployment observations. Validation belongs to
// the readiness owner because missing values are warning/failure states rather
// than process-startup configuration errors.
type Release struct {
	Environment string
	SHA         string
}

// LegacyHealthSnapshot carries the presence/mode facts behind the public
// LEGACY-API-0757 GET /health runtime-mode snapshot. Legacy configuration
// names are folded into booleans at load time; Root never stores, logs, or
// returns their raw values. DatabaseIsPostgres is set where the startup
// database URL is validated, because AICRM_DATABASE_URL only accepts
// PostgreSQL in v2.
type LegacyHealthSnapshot struct {
	DatabaseIsPostgres                  bool
	ProductionEnvironment               bool
	SecretKeyPresent                    bool
	WeChatShopCallbackTokenPresent      bool
	AllowMissingWeChatShopCallbackToken bool
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

type OAuthSecret struct{ value string }

func (secret OAuthSecret) Value() string { return secret.value }
func (OAuthSecret) String() string       { return "[REDACTED]" }
func (OAuthSecret) GoString() string     { return "[REDACTED]" }

// WeComOAuth is optional as a whole so code acceptance can run without a live
// provider. Any partial production configuration is rejected at startup.
type WeComOAuth struct {
	Enabled     bool
	CorpID      string
	Secret      OAuthSecret
	CallbackURL string
}

// WeComDirectorySync is explicitly opt-in. The staff list is the complete
// eligible scope for the provider's external-contact directory read; an empty
// configuration never schedules a provider job.
type WeComDirectorySync struct {
	Enabled      bool
	StaffUserIDs []string
}

// WeComSidebar is the independent OAuth and JSSDK configuration for the
// embedded sidebar. It deliberately cannot reuse the administrator OAuth
// callback, because the two browser protocols have different bindings.
type WeComSidebar struct {
	Enabled      bool
	CorpID       string
	Secret       OAuthSecret
	CallbackURL  string
	AgentID      int64
	AllowedHosts []string
}

type WeCom struct {
	Callback      WeComCallback
	OAuth         WeComOAuth
	DirectorySync WeComDirectorySync
	Sidebar       WeComSidebar
}

type IdentityHMACKey struct{ value [32]byte }

func (key IdentityHMACKey) Value() []byte { return append([]byte(nil), key.value[:]...) }
func (IdentityHMACKey) String() string    { return "[REDACTED]" }
func (IdentityHMACKey) GoString() string  { return "[REDACTED]" }

type Identity struct {
	HMACKey IdentityHMACKey
}

// APIClientJWTSecret is optional startup-only key material for the bounded
// external API-client protocol. When absent, those routes remain fail-closed.
type APIClientJWTSecret struct {
	value      [32]byte
	configured bool
}

func (key APIClientJWTSecret) Value() []byte    { return append([]byte(nil), key.value[:]...) }
func (key APIClientJWTSecret) Configured() bool { return key.configured }
func (APIClientJWTSecret) String() string       { return "[REDACTED]" }
func (APIClientJWTSecret) GoString() string     { return "[REDACTED]" }

type APIClient struct {
	JWTSecret APIClientJWTSecret
}

// SurveyPublicKey is optional as a whole. A missing key leaves every public
// Survey operation fail-closed with 503, while an invalid configured value
// rejects process startup. Generic formatting can never reveal the key.
type SurveyPublicKey struct{ value [32]byte }

func (key SurveyPublicKey) Value() []byte { return append([]byte(nil), key.value[:]...) }
func (SurveyPublicKey) String() string    { return "[REDACTED]" }
func (SurveyPublicKey) GoString() string  { return "[REDACTED]" }

type Survey struct {
	PublicKey SurveyPublicKey
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
	Database           Database
	API                API
	Worker             Worker
	WeCom              WeCom
	Identity           Identity
	APIClient          APIClient
	Survey             Survey
	DomainVerification DomainVerification
	Release            Release
	LegacyHealth       LegacyHealthSnapshot
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
		root.LegacyHealth.DatabaseIsPostgres = true
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
		root.WeCom.OAuth = parseWeComOAuth(lookup, &problems)
		root.WeCom.DirectorySync = parseWeComDirectorySync(lookup, &problems)
		if root.WeCom.DirectorySync.Enabled && !root.WeCom.OAuth.Enabled {
			problems = append(problems, "wecom.directory_sync requires configured oauth credentials")
		}
		root.WeCom.Sidebar = parseWeComSidebar(lookup, &problems)
		root.Identity.HMACKey = parseIdentityHMACKey(lookup, &problems)
		root.APIClient.JWTSecret = parseOptionalAPIClientJWTSecret(lookup, &problems)
		root.Survey.PublicKey = parseOptionalSurveyPublicKey(lookup, &problems)
		root.DomainVerification.Directory, _ = lookup(domainVerificationDirEnv)
		root.Release.Environment, _ = lookup(applicationEnvironmentEnv)
		root.Release.SHA, _ = lookup(releaseSHAEnv)
		root.LegacyHealth.SecretKeyPresent = legacyValuePresent(lookup, legacySecretKeyEnv)
		root.LegacyHealth.WeChatShopCallbackTokenPresent = legacyValuePresent(lookup, legacyWeChatShopCallbackTokenEnv)
		root.LegacyHealth.AllowMissingWeChatShopCallbackToken = legacyToggleEnabled(lookup, legacyAllowMissingWeChatShopCallbackToken)
		root.LegacyHealth.ProductionEnvironment = legacyProductionEnvironment(lookup)
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
		if !needAPI {
			root.WeCom.DirectorySync = parseWeComDirectorySync(lookup, &problems)
			if root.WeCom.DirectorySync.Enabled {
				root.WeCom.OAuth = parseWeComOAuth(lookup, &problems)
				if !root.WeCom.OAuth.Enabled {
					problems = append(problems, "wecom.directory_sync requires configured oauth credentials")
				}
				root.Identity.HMACKey = parseIdentityHMACKey(lookup, &problems)
			}
		}
	}
	if len(problems) != 0 {
		return Root{}, validationError{problems: problems}
	}
	return root, nil
}

func parseIdentityHMACKey(lookup environmentLookup, problems *[]string) IdentityHMACKey {
	value, present := lookup(identityHMACKeyEnv)
	if !present || value == "" {
		*problems = append(*problems, "identity.hmac_key is required")
		return IdentityHMACKey{}
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		*problems = append(*problems, "identity.hmac_key must be 32-byte canonical base64url")
		return IdentityHMACKey{}
	}
	var key IdentityHMACKey
	copy(key.value[:], decoded)
	return key
}

func parseOptionalSurveyPublicKey(lookup environmentLookup, problems *[]string) SurveyPublicKey {
	value, present := lookup(surveyPublicKeyEnv)
	if !present || value == "" {
		return SurveyPublicKey{}
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		*problems = append(*problems, "survey.public_key must be 32-byte canonical base64url")
		return SurveyPublicKey{}
	}
	var key SurveyPublicKey
	copy(key.value[:], decoded)
	return key
}

func parseOptionalAPIClientJWTSecret(lookup environmentLookup, problems *[]string) APIClientJWTSecret {
	value, present := lookup(apiClientJWTSecretEnv)
	if !present || value == "" {
		return APIClientJWTSecret{}
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		*problems = append(*problems, "api_client.jwt_secret must be 32-byte canonical base64url")
		return APIClientJWTSecret{}
	}
	var key APIClientJWTSecret
	copy(key.value[:], decoded)
	key.configured = true
	return key
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

func parseWeComOAuth(lookup environmentLookup, problems *[]string) WeComOAuth {
	corpID, corpIDPresent := lookup(weComOAuthCorpIDEnv)
	secret, secretPresent := lookup(weComOAuthSecretEnv)
	callbackURL, callbackPresent := lookup(weComOAuthCallbackEnv)
	if !corpIDPresent && !secretPresent && !callbackPresent {
		return WeComOAuth{}
	}
	if !corpIDPresent || !secretPresent || !callbackPresent || corpID == "" || secret == "" || callbackURL == "" {
		*problems = append(*problems, "wecom.oauth requires corp_id, secret, and callback_url together")
		return WeComOAuth{}
	}
	if !validWeComCorpID(corpID) {
		*problems = append(*problems, "wecom.oauth.corp_id is invalid")
	}
	if len(secret) > 256 || strings.TrimSpace(secret) != secret {
		*problems = append(*problems, "wecom.oauth.secret is invalid")
	}
	if !validOAuthCallbackURL(callbackURL) {
		*problems = append(*problems, "wecom.oauth.callback_url is invalid")
	}
	return WeComOAuth{Enabled: true, CorpID: corpID, Secret: OAuthSecret{value: secret}, CallbackURL: callbackURL}
}

func parseWeComDirectorySync(lookup environmentLookup, problems *[]string) WeComDirectorySync {
	enabled, present := lookup(weComDirectorySyncEnabledEnv)
	staffUserIDs, staffPresent := lookup(weComDirectorySyncStaffUserIDsEnv)
	if !present && !staffPresent {
		return WeComDirectorySync{}
	}
	if !present || (enabled != "true" && enabled != "false") {
		*problems = append(*problems, "wecom.directory_sync.enabled must be true or false")
		return WeComDirectorySync{}
	}
	if enabled == "false" {
		if staffPresent && staffUserIDs != "" {
			*problems = append(*problems, "wecom.directory_sync.staff_user_ids requires enabled=true")
		}
		return WeComDirectorySync{}
	}
	if !staffPresent || staffUserIDs == "" {
		*problems = append(*problems, "wecom.directory_sync.staff_user_ids is required when enabled")
		return WeComDirectorySync{}
	}
	parts := strings.Split(staffUserIDs, ",")
	if len(parts) > 64 {
		*problems = append(*problems, "wecom.directory_sync.staff_user_ids is invalid")
		return WeComDirectorySync{}
	}
	seen := make(map[string]struct{}, len(parts))
	for _, staffUserID := range parts {
		if !validWeComDirectorySyncStaffUserID(staffUserID) {
			*problems = append(*problems, "wecom.directory_sync.staff_user_ids is invalid")
			return WeComDirectorySync{}
		}
		if _, duplicate := seen[staffUserID]; duplicate {
			*problems = append(*problems, "wecom.directory_sync.staff_user_ids is invalid")
			return WeComDirectorySync{}
		}
		seen[staffUserID] = struct{}{}
	}
	return WeComDirectorySync{Enabled: true, StaffUserIDs: parts}
}

func validWeComDirectorySyncStaffUserID(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

func validOAuthCallbackURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Opaque == "" &&
		parsed.Path == "/auth/wecom/callback" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func parseWeComSidebar(lookup environmentLookup, problems *[]string) WeComSidebar {
	corpID, corpIDPresent := lookup(weComSidebarCorpIDEnv)
	secret, secretPresent := lookup(weComSidebarSecretEnv)
	callbackURL, callbackPresent := lookup(weComSidebarCallbackEnv)
	agentID, agentIDPresent := lookup(weComSidebarAgentIDEnv)
	hosts, hostsPresent := lookup(weComSidebarHostsEnv)
	if !corpIDPresent && !secretPresent && !callbackPresent && !agentIDPresent && !hostsPresent {
		return WeComSidebar{}
	}
	if !corpIDPresent || !secretPresent || !callbackPresent || !agentIDPresent || !hostsPresent || corpID == "" || secret == "" || callbackURL == "" || agentID == "" || hosts == "" {
		*problems = append(*problems, "wecom.sidebar requires corp_id, secret, callback_url, agent_id, and allowed_hosts together")
		return WeComSidebar{}
	}
	if !validWeComCorpID(corpID) {
		*problems = append(*problems, "wecom.sidebar.corp_id is invalid")
	}
	if len(secret) > 256 || strings.TrimSpace(secret) != secret {
		*problems = append(*problems, "wecom.sidebar.secret is invalid")
	}
	if !validSidebarCallbackURL(callbackURL) {
		*problems = append(*problems, "wecom.sidebar.callback_url is invalid")
	}
	parsedAgentID, err := strconv.ParseInt(agentID, 10, 64)
	if err != nil || parsedAgentID < 1 || strconv.FormatInt(parsedAgentID, 10) != agentID {
		*problems = append(*problems, "wecom.sidebar.agent_id is invalid")
	}
	allowedHosts, validHosts := parseSidebarHosts(hosts)
	if !validHosts {
		*problems = append(*problems, "wecom.sidebar.allowed_hosts is invalid")
	}
	return WeComSidebar{Enabled: true, CorpID: corpID, Secret: OAuthSecret{value: secret}, CallbackURL: callbackURL, AgentID: parsedAgentID, AllowedHosts: allowedHosts}
}

func validSidebarCallbackURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Opaque == "" &&
		parsed.Path == "/api/sidebar/v2/oauth/callback" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func parseSidebarHosts(value string) ([]string, bool) {
	if strings.TrimSpace(value) != value {
		return nil, false
	}
	parts := strings.Split(value, ",")
	if len(parts) < 1 || len(parts) > 16 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if !validSidebarHost(part) {
			return nil, false
		}
		if _, duplicate := seen[part]; duplicate {
			return nil, false
		}
		seen[part] = struct{}{}
	}
	return parts, true
}

func validSidebarHost(value string) bool {
	if len(value) < 1 || len(value) > 253 || strings.ToLower(value) != value || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
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

// legacyValuePresent folds a legacy presence-only name into a boolean. The raw
// value is discarded immediately so the health snapshot cannot expose secrets.
func legacyValuePresent(lookup environmentLookup, name string) bool {
	value, present := lookup(name)
	return present && value != ""
}

// legacyToggleEnabled accepts exactly the legacy truthy tokens 1/true/yes/on,
// trimmed and case-insensitive; everything else stays false.
func legacyToggleEnabled(lookup environmentLookup, name string) bool {
	value, present := lookup(name)
	if !present {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// legacyProductionEnvironment reports true when any legacy production alias
// holds prod or production, trimmed and case-insensitive.
func legacyProductionEnvironment(lookup environmentLookup) bool {
	for _, name := range legacyProductionEnvironmentEnvs {
		value, present := lookup(name)
		if !present {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "prod", "production":
			return true
		}
	}
	return false
}

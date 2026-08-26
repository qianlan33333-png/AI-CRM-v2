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
	weChatPayEnabledEnv               = "AICRM_WECHAT_PAY_ENABLED"
	weChatPayAppIDEnv                 = "AICRM_WECHAT_PAY_APP_ID"
	weChatPayMerchantIDEnv            = "AICRM_WECHAT_PAY_MERCHANT_ID"
	weChatPayMerchantSerialEnv        = "AICRM_WECHAT_PAY_MERCHANT_SERIAL"
	weChatPayMerchantPrivateKeyEnv    = "AICRM_WECHAT_PAY_MERCHANT_PRIVATE_KEY"
	weChatPayAPIV3KeyEnv              = "AICRM_WECHAT_PAY_API_V3_KEY"
	weChatPayPlatformSerialEnv        = "AICRM_WECHAT_PAY_PLATFORM_SERIAL"
	weChatPayPlatformCertificateEnv   = "AICRM_WECHAT_PAY_PLATFORM_CERTIFICATE"
	weChatPayPaymentNotifyURLEnv      = "AICRM_WECHAT_PAY_PAYMENT_NOTIFY_URL"
	weChatPayRefundNotifyURLEnv       = "AICRM_WECHAT_PAY_REFUND_NOTIFY_URL"
	weChatPayPermissionConfirmedEnv   = "AICRM_WECHAT_PAY_PERMISSION_CONFIRMED"

	legacySecretKeyEnv                        = "SECRET_KEY"
	legacyWeChatShopCallbackTokenEnv          = "WECHAT_SHOP_CALLBACK_TOKEN"
	legacyAllowMissingWeChatShopCallbackToken = "AICRM_ALLOW_MISSING_WECHAT_SHOP_CALLBACK_TOKEN"
)

const (
	weComCustomerAcquisitionEnabledEnv             = "AICRM_WECOM_CUSTOMER_ACQUISITION_ENABLED"
	weComCustomerAcquisitionCorpIDEnv              = "AICRM_WECOM_CUSTOMER_ACQUISITION_CORP_ID"
	weComCustomerAcquisitionSecretEnv              = "AICRM_WECOM_CUSTOMER_ACQUISITION_SECRET"
	weComCustomerAcquisitionPermissionConfirmedEnv = "AICRM_WECOM_CUSTOMER_ACQUISITION_PERMISSION_CONFIRMED"
	weComOutboundEnabledEnv                        = "AICRM_WECOM_OUTBOUND_ENABLED"
	weComOutboundCorpIDEnv                         = "AICRM_WECOM_OUTBOUND_CORP_ID"
	weComOutboundSecretEnv                         = "AICRM_WECOM_OUTBOUND_SECRET"
	weComOutboundPermissionConfirmedEnv            = "AICRM_WECOM_OUTBOUND_PERMISSION_CONFIRMED"
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

// CustomerAcquisitionSecret is the credential of the explicitly authorized
// CH02 application. It is not the Sidebar, callback, or administrator OAuth
// secret, and formatting can never expose its value.
type CustomerAcquisitionSecret struct{ value string }

func (secret CustomerAcquisitionSecret) Value() string { return secret.value }
func (CustomerAcquisitionSecret) String() string       { return "[REDACTED]" }
func (CustomerAcquisitionSecret) GoString() string     { return "[REDACTED]" }

// WeComCustomerAcquisition is opt-in and requires an operator assertion that
// this independent application has the customer-acquisition write permission.
// Disabled configuration creates no token provider, HTTP client, worker, job,
// or recovery schedule.
type WeComCustomerAcquisition struct {
	Enabled             bool
	CorpID              string
	Secret              CustomerAcquisitionSecret
	PermissionConfirmed bool
}

type WeComOutboundSecret struct{ value string }

func (secret WeComOutboundSecret) Value() string { return secret.value }
func (WeComOutboundSecret) String() string       { return "[REDACTED]" }
func (WeComOutboundSecret) GoString() string     { return "[REDACTED]" }

// WeComOutbound is an independent, worker-only opt-in for creating external-
// contact message templates. It cannot inherit OAuth, Sidebar, callback, or
// customer-acquisition credentials.
type WeComOutbound struct {
	Enabled             bool
	CorpID              string
	Secret              WeComOutboundSecret
	PermissionConfirmed bool
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
	Callback            WeComCallback
	OAuth               WeComOAuth
	DirectorySync       WeComDirectorySync
	Sidebar             WeComSidebar
	CustomerAcquisition WeComCustomerAcquisition
	Outbound            WeComOutbound
}

// CommerceProviderSecret is intentionally opaque. Payment credentials must
// never surface through config formatting or startup error text.
type CommerceProviderSecret struct{ value string }

func (secret CommerceProviderSecret) Value() string { return secret.value }
func (CommerceProviderSecret) String() string       { return "[REDACTED]" }
func (CommerceProviderSecret) GoString() string     { return "[REDACTED]" }

// WeChatPayProvider is role-scoped: API loads only callback verification
// material, while Worker loads only outbound payment credentials and requires
// an explicit permission assertion.
type WeChatPayProvider struct {
	Enabled             bool
	AppID               string
	MerchantID          string
	MerchantSerial      string
	MerchantPrivateKey  CommerceProviderSecret
	APIV3Key            CommerceProviderSecret
	PlatformSerial      string
	PlatformCertificate CommerceProviderSecret
	PaymentNotifyURL    string
	RefundNotifyURL     string
	PermissionConfirmed bool
}

type CommerceProviders struct {
	WeChatPay WeChatPayProvider
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
	Commerce           CommerceProviders
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
	if needAPI || needWorker {
		root.Commerce.WeChatPay = parseWeChatPayProvider(lookup, needAPI, needWorker, &problems)
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
		root.WeCom.CustomerAcquisition = parseWeComCustomerAcquisition(lookup, &problems)
		root.WeCom.Outbound = parseWeComOutbound(lookup, &problems)
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

func parseWeComCustomerAcquisition(lookup environmentLookup, problems *[]string) WeComCustomerAcquisition {
	enabled, enabledPresent := lookup(weComCustomerAcquisitionEnabledEnv)
	corpID, corpIDPresent := lookup(weComCustomerAcquisitionCorpIDEnv)
	secret, secretPresent := lookup(weComCustomerAcquisitionSecretEnv)
	permission, permissionPresent := lookup(weComCustomerAcquisitionPermissionConfirmedEnv)
	if !enabledPresent && !corpIDPresent && !secretPresent && !permissionPresent {
		return WeComCustomerAcquisition{}
	}
	if !enabledPresent || enabled != "true" && enabled != "false" {
		*problems = append(*problems, "wecom.customer_acquisition.enabled must be true or false")
		return WeComCustomerAcquisition{}
	}
	if enabled == "false" {
		if corpIDPresent || secretPresent || permissionPresent {
			*problems = append(*problems, "wecom.customer_acquisition credentials require enabled=true")
		}
		return WeComCustomerAcquisition{}
	}
	if !corpIDPresent || !secretPresent || !permissionPresent || corpID == "" || secret == "" {
		*problems = append(*problems, "wecom.customer_acquisition requires corp_id, secret, and permission_confirmed together")
		return WeComCustomerAcquisition{}
	}
	if !validWeComCorpID(corpID) {
		*problems = append(*problems, "wecom.customer_acquisition.corp_id is invalid")
	}
	if len(secret) > 256 || strings.TrimSpace(secret) != secret {
		*problems = append(*problems, "wecom.customer_acquisition.secret is invalid")
	}
	if permission != "true" {
		*problems = append(*problems, "wecom.customer_acquisition.permission_confirmed must be true when enabled")
	}
	return WeComCustomerAcquisition{
		Enabled: true, CorpID: corpID, Secret: CustomerAcquisitionSecret{value: secret}, PermissionConfirmed: permission == "true",
	}
}

func parseWeComOutbound(lookup environmentLookup, problems *[]string) WeComOutbound {
	enabled, enabledPresent := lookup(weComOutboundEnabledEnv)
	corpID, corpIDPresent := lookup(weComOutboundCorpIDEnv)
	secret, secretPresent := lookup(weComOutboundSecretEnv)
	permission, permissionPresent := lookup(weComOutboundPermissionConfirmedEnv)
	if !enabledPresent && !corpIDPresent && !secretPresent && !permissionPresent {
		return WeComOutbound{}
	}
	if !enabledPresent || enabled != "true" && enabled != "false" {
		*problems = append(*problems, "wecom.outbound.enabled must be true or false")
		return WeComOutbound{}
	}
	if enabled == "false" {
		if corpIDPresent || secretPresent || permissionPresent {
			*problems = append(*problems, "wecom.outbound credentials require enabled=true")
		}
		return WeComOutbound{}
	}
	if !corpIDPresent || !secretPresent || !permissionPresent || corpID == "" || secret == "" {
		*problems = append(*problems, "wecom.outbound requires corp_id, secret, and permission_confirmed together")
		return WeComOutbound{}
	}
	if !validWeComCorpID(corpID) {
		*problems = append(*problems, "wecom.outbound.corp_id is invalid")
	}
	if len(secret) > 256 || strings.TrimSpace(secret) != secret {
		*problems = append(*problems, "wecom.outbound.secret is invalid")
	}
	if permission != "true" {
		*problems = append(*problems, "wecom.outbound.permission_confirmed must be true when enabled")
	}
	return WeComOutbound{Enabled: true, CorpID: corpID, Secret: WeComOutboundSecret{value: secret}, PermissionConfirmed: permission == "true"}
}

func parseWeChatPayProvider(lookup environmentLookup, needAPI, needWorker bool, problems *[]string) WeChatPayProvider {
	enabled, enabledPresent := lookup(weChatPayEnabledEnv)
	appID, appIDPresent := lookup(weChatPayAppIDEnv)
	merchantID, merchantIDPresent := lookup(weChatPayMerchantIDEnv)
	platformSerial, platformSerialPresent := lookup(weChatPayPlatformSerialEnv)
	platformCertificate, platformCertificatePresent := lookup(weChatPayPlatformCertificateEnv)
	var merchantSerial, merchantKey, paymentNotifyURL, refundNotifyURL, permission string
	var merchantSerialPresent, merchantKeyPresent, paymentNotifyPresent, refundNotifyPresent, permissionPresent bool
	if needWorker {
		merchantSerial, merchantSerialPresent = lookup(weChatPayMerchantSerialEnv)
		merchantKey, merchantKeyPresent = lookup(weChatPayMerchantPrivateKeyEnv)
		paymentNotifyURL, paymentNotifyPresent = lookup(weChatPayPaymentNotifyURLEnv)
		refundNotifyURL, refundNotifyPresent = lookup(weChatPayRefundNotifyURLEnv)
		permission, permissionPresent = lookup(weChatPayPermissionConfirmedEnv)
	}
	var apiV3Key string
	var apiV3KeySet bool
	if needAPI {
		apiV3Key, apiV3KeySet = lookup(weChatPayAPIV3KeyEnv)
	}
	anyPresent := enabledPresent || appIDPresent || merchantIDPresent || platformSerialPresent || platformCertificatePresent || merchantSerialPresent || merchantKeyPresent || paymentNotifyPresent || refundNotifyPresent || permissionPresent || apiV3KeySet
	if !anyPresent {
		return WeChatPayProvider{}
	}
	if !enabledPresent || (enabled != "true" && enabled != "false") {
		*problems = append(*problems, "wechat_pay.enabled must be true or false")
		return WeChatPayProvider{}
	}
	if enabled == "false" {
		if anyPresent && (appIDPresent || merchantIDPresent || platformSerialPresent || platformCertificatePresent || merchantSerialPresent || merchantKeyPresent || paymentNotifyPresent || refundNotifyPresent || permissionPresent || apiV3KeySet) {
			*problems = append(*problems, "wechat_pay credentials require enabled=true")
		}
		return WeChatPayProvider{}
	}
	if !appIDPresent || !merchantIDPresent || !platformSerialPresent || !platformCertificatePresent || appID == "" || merchantID == "" || platformSerial == "" || platformCertificate == "" {
		*problems = append(*problems, "wechat_pay requires app_id, merchant_id, platform_serial, and platform_certificate")
		return WeChatPayProvider{}
	}
	if needAPI && (!apiV3KeySet || apiV3Key == "") {
		*problems = append(*problems, "wechat_pay callback requires api_v3_key")
		return WeChatPayProvider{}
	}
	if needWorker && (!merchantSerialPresent || !merchantKeyPresent || !paymentNotifyPresent || !refundNotifyPresent || !permissionPresent || merchantSerial == "" || merchantKey == "" || paymentNotifyURL == "" || refundNotifyURL == "") {
		*problems = append(*problems, "wechat_pay worker requires merchant_serial, merchant_private_key, payment_notify_url, refund_notify_url, and permission_confirmed")
		return WeChatPayProvider{}
	}
	if !validProviderIdentifier(appID) || !validProviderIdentifier(merchantID) || !validProviderIdentifier(platformSerial) || (needWorker && !validProviderIdentifier(merchantSerial)) {
		*problems = append(*problems, "wechat_pay identifiers are invalid")
	}
	if (needWorker && !validOpaqueProviderSecret(merchantKey)) || (needAPI && (len(apiV3Key) != 32 || !validOpaqueProviderSecret(apiV3Key))) || !validOpaqueProviderSecret(platformCertificate) {
		*problems = append(*problems, "wechat_pay credentials are invalid")
	}
	if needWorker && (!validProviderCallbackURL(paymentNotifyURL) || !validProviderCallbackURL(refundNotifyURL) || paymentNotifyURL == refundNotifyURL) {
		*problems = append(*problems, "wechat_pay notify urls are invalid")
	}
	if needWorker && permission != "true" {
		*problems = append(*problems, "wechat_pay.permission_confirmed must be true when enabled")
	}
	return WeChatPayProvider{Enabled: true, AppID: appID, MerchantID: merchantID, MerchantSerial: merchantSerial, MerchantPrivateKey: CommerceProviderSecret{value: merchantKey}, APIV3Key: CommerceProviderSecret{value: apiV3Key}, PlatformSerial: platformSerial, PlatformCertificate: CommerceProviderSecret{value: platformCertificate}, PaymentNotifyURL: paymentNotifyURL, RefundNotifyURL: refundNotifyURL, PermissionConfirmed: permission == "true"}
}

func validProviderIdentifier(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func validOpaqueProviderSecret(value string) bool {
	return value != "" && len(value) <= 16<<10 && strings.TrimSpace(value) == value
}

func validProviderCallbackURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Opaque == "" && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Path != ""
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

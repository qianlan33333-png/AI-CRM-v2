// Package legacyhealth implements the frozen leaf for LEGACY-API-0757.
//
// The central v2 router owns the public route table, OpenAPI contract, and
// generated bindings; this package owns only the exact legacy payload and
// method semantics.
package legacyhealth

import (
	"encoding/json"
	"net/http"
)

const contentTypeJSON = "application/json"

// RuntimeSnapshot contains presence and mode facts only. It intentionally
// carries no secret material or database URL, so the health DTO cannot expose
// credentials when it is serialized.
type RuntimeSnapshot struct {
	DatabaseIsPostgres                  bool
	ProductionEnvironment               bool
	SecretKeyPresent                    bool
	WeChatShopCallbackTokenPresent      bool
	AllowMissingWeChatShopCallbackToken bool
}

// Payload is the exact user-visible JSON object returned by the legacy
// GET /health handler at immutable legacy SHA 6cb989c.
type Payload struct {
	OK                              bool   `json:"ok"`
	Status                          string `json:"status"`
	Service                         string `json:"service"`
	SecretKeyPresent                bool   `json:"secret_key_present"`
	WeChatShopCallbackTokenPresent  bool   `json:"wechat_shop_callback_token_present"`
	WeChatShopCallbackTokenRequired bool   `json:"wechat_shop_callback_token_required"`
	Database                        string `json:"database"`
	DatabaseMode                    string `json:"database_mode"`
	FixtureMode                     bool   `json:"fixture_mode"`
	ProductionDataReady             bool   `json:"production_data_ready"`
	ProductionDataMode              bool   `json:"production_data_mode"`
	RepositoryPolicy                string `json:"repository_policy"`
	RuntimeOwner                    string `json:"runtime_owner"`
	LegacyRuntimeEnabled            bool   `json:"legacy_runtime_enabled"`
	Warning                         string `json:"warning"`
}

// Query is the legacy GetSystemHealthQuery equivalent, kept independent from
// HTTP routing and from any database or provider call.
type Query struct {
	snapshot RuntimeSnapshot
}

func NewQuery(snapshot RuntimeSnapshot) Query {
	return Query{snapshot: snapshot}
}

func (query Query) Execute() Payload {
	fixtureMode := !query.snapshot.DatabaseIsPostgres
	productionDataReady := query.snapshot.DatabaseIsPostgres
	degraded := fixtureMode && query.snapshot.ProductionEnvironment
	status := "ok"
	if degraded {
		status = "degraded"
	}
	warning := ""
	if degraded {
		warning = "production runtime is using fixture data; production data is not ready"
	} else if fixtureMode {
		warning = "fixture data mode"
	}
	databaseMode := "fixture"
	if query.snapshot.DatabaseIsPostgres {
		databaseMode = "postgres"
	}
	repositoryPolicy := "fixture_repositories_allowed"
	if query.snapshot.DatabaseIsPostgres || query.snapshot.ProductionEnvironment {
		repositoryPolicy = "production_repositories_required"
	}
	return Payload{
		OK:                              !degraded,
		Status:                          status,
		Service:                         "aicrm-next",
		SecretKeyPresent:                query.snapshot.SecretKeyPresent,
		WeChatShopCallbackTokenPresent:  query.snapshot.WeChatShopCallbackTokenPresent,
		WeChatShopCallbackTokenRequired: query.snapshot.ProductionEnvironment && !query.snapshot.AllowMissingWeChatShopCallbackToken,
		Database:                        databaseMode,
		DatabaseMode:                    databaseMode,
		FixtureMode:                     fixtureMode,
		ProductionDataReady:             productionDataReady,
		ProductionDataMode:              productionDataReady,
		RepositoryPolicy:                repositoryPolicy,
		RuntimeOwner:                    "ai_crm_next",
		LegacyRuntimeEnabled:            false,
		Warning:                         warning,
	}
}

// Handler serves the legacy GET /health payload. The central v2 router mounts
// it for every method so the exact legacy 405 guard below stays authoritative.
type Handler struct {
	query Query
}

func NewHandler(query Query) *Handler {
	return &Handler{query: query}
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", contentTypeJSON)
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writer.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = writer.Write([]byte(`{"detail":"Method Not Allowed"}`))
		return
	}
	payload, _ := json.Marshal(handler.query.Execute())
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(payload)
}

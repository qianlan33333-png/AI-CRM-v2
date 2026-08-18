package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	contractapi "github.com/qianlan33333-png/AI-CRM-v2/api"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

// ---- frozen table unit tests ----------------------------------------------

func TestLegacyAPIDocsAllowedFreezesFirstLevelAllowPattern(t *testing.T) {
	tests := []struct {
		path    string
		allowed bool
	}{
		{"/health", true},
		{"/mcp", true},
		{"/login", true},
		{"/logout", true},
		{"/api/customers", true},
		{"/api/admin/image-library", true},
		{"/wecom/callback", true},
		{"/auth/wecom/callback", true},
		{"/p/abc", true},
		{"/pay/notify", true},
		{"/s/xyz", true},
		{"/admin", false},
		{"/admin/api-docs", false},
		{"/admin/config/mcp-tools", false},
		{"/static/app.js", false},
		{"/healthz", false},
		{"/api", false},
		{"/internal/probe", false},
		{"/c/public", false},
	}
	for _, test := range tests {
		if got := legacyAPIDocsAllowed(test.path); got != test.allowed {
			t.Fatalf("legacyAPIDocsAllowed(%q) = %v, want %v", test.path, got, test.allowed)
		}
	}
}

func TestLegacyAPIDocsGroupIDFreezesFirstMatchOrder(t *testing.T) {
	tests := []struct {
		path  string
		group string
	}{
		{"/health", "system-mcp"},
		{"/api/system/health", "system-mcp"},
		{"/api/frontend-compat/anything", "system-mcp"},
		{"/api/admin/exports", "system-mcp"},
		{"/api/admin/exports/1", "system-mcp"},
		{"/login", "auth-callback"},
		{"/wecom/callback", "auth-callback"},
		{"/api/wecom/events", "auth-callback"},
		{"/api/admin/webhooks/wecom", "auth-callback"},
		{"/pay/wechat/notify", "auth-callback"},
		{"/api/customers", "customer-identity-sidebar"},
		{"/api/sidebar/v2/coupons", "customer-identity-sidebar"},
		{"/api/admin/customers/1", "customer-identity-sidebar"},
		{"/api/admin/channels", "channels"},
		{"/api/admin/questionnaires", "questionnaires"},
		{"/api/h5/questionnaires/1", "questionnaires"},
		{"/api/admin/user-ops/board", "user-ops"},
		{"/api/admin/automation-conversion/group-ops/plans", "group-ops"},
		{"/api/automation/group-ops/webhook", "group-ops"},
		{"/api/admin/wecom/tags", "wecom-tags"},
		{"/api/admin/image-library", "materials-send-content"},
		{"/api/admin/send-content/validate", "materials-send-content"},
		{"/api/admin/wechat-pay/orders", "commerce"},
		{"/p/abc", "commerce"},
		{"/api/admin/ai-assist/chat", "ai-assist-compat"},
		{"/api/admin/push-center/stats", "push-center"},
		{"/api/admin/external-effects/jobs", "external-effects"},
		{"/api/admin/automation-conversion/rules", "automation"},
		{"/api/unknown-thing", "other"},
	}
	for _, test := range tests {
		if got := legacyAPIDocsGroupID(test.path); got != test.group {
			t.Fatalf("legacyAPIDocsGroupID(%q) = %q, want %q", test.path, got, test.group)
		}
	}
}

func TestLegacyAPIDocsSlug(t *testing.T) {
	tests := []struct {
		path string
		slug string
	}{
		{"/api/customers/{customer_id}", "api-customers-customer-id"},
		{"/api/v1/order_board", "api-v1-order-board"},
		{"/api/CMS-Pages", "api-cms-pages"},
		{"/", "endpoint"},
		{"/api/things-list", "api-things-list"},
	}
	for _, test := range tests {
		if got := legacyAPIDocsSlug(test.path); got != test.slug {
			t.Fatalf("legacyAPIDocsSlug(%q) = %q, want %q", test.path, got, test.slug)
		}
	}
}

func TestLegacyAPIDocsStripParamTypes(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"/api/orders/{order_id:int}", "/api/orders/{order_id}"},
		{"/api/x/{a}/{b:uuid}", "/api/x/{a}/{b}"},
		{"/plain", "/plain"},
	}
	for _, test := range tests {
		if got := legacyAPIDocsStripParamTypes(test.raw); got != test.want {
			t.Fatalf("legacyAPIDocsStripParamTypes(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

// ---- builder tests ---------------------------------------------------------

const legacyAPIDocsTestSpecHead = `openapi: 3.1.0
info:
  title: synthetic
  version: v1
security:
  - humanSession: []
`

func TestLegacyAPIDocsModelBuildsSafeProjection(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /health:
    get:
      summary: 健康检查
      responses:
        "200":
          description: ok
  /api/things:
    parameters:
      - name: X-Trace
        in: header
        required: false
        schema:
          type: string
    get:
      summary: 事物列表
      description: 返回全部事物。
      x-aicrm-capability: things.read
      x-aicrm-session-bound-csrf: none
      x-aicrm-external-effect: none
      parameters:
        - name: page
          in: query
          required: true
          schema:
            type: integer
            format: int64
            minimum: 1
            maximum: 200
        - $ref: "#/components/parameters/Cursor"
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ThingList"
        "422":
          $ref: "#/components/responses/ValidationError"
    post:
      summary: 创建事物
      security: []
      responses:
        "204":
          description: done
components:
  parameters:
    Cursor:
      name: cursor
      in: query
      required: false
      schema:
        type: string
  responses:
    ValidationError:
      description: 校验失败
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/ErrorEnvelope"
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	if model.EndpointCount != 3 || len(model.QuickReference) != 3 {
		t.Fatalf("endpoint_count=%d quick=%d", model.EndpointCount, len(model.QuickReference))
	}
	if len(model.Groups) != 2 || model.Groups[0].ID != "system-mcp" || model.Groups[1].ID != "other" {
		t.Fatalf("groups=%v", model.Groups)
	}

	things := model.Groups[1]
	if len(things.Endpoints) != 2 {
		t.Fatalf("things endpoints=%d", len(things.Endpoints))
	}
	get := things.Endpoints[0]
	if get.ID != "get-api-things" || get.Method != http.MethodGet || get.MethodLower != "get" || get.Path != "/api/things" {
		t.Fatalf("get endpoint=%+v", get)
	}
	if get.Summary != "事物列表" || get.Description != "返回全部事物。" {
		t.Fatalf("summary=%q description=%q", get.Summary, get.Description)
	}
	if get.AuthLabel != "humanSession" || get.Capability != "things.read" || get.CSRF != "none" || get.Effect != "none" {
		t.Fatalf("auth=%q capability=%q csrf=%q effect=%q", get.AuthLabel, get.Capability, get.CSRF, get.Effect)
	}
	if len(get.Params) != 3 {
		t.Fatalf("params=%+v", get.Params)
	}
	if get.Params[0].Name != "X-Trace" || get.Params[0].Location != "header" || get.Params[0].Type != "string" {
		t.Fatalf("shared param=%+v", get.Params[0])
	}
	page := get.Params[1]
	if page.Name != "page" || page.Location != "query" || !page.Required || page.Type != "integer" || page.Format != "int64" || page.Range != "min=1 max=200" {
		t.Fatalf("page param=%+v", page)
	}
	if get.Params[2].Name != "cursor" || get.Params[2].Location != "query" {
		t.Fatalf("cursor param=%+v", get.Params[2])
	}
	if len(get.Responses) != 2 || get.Responses[0].Status != "200" || get.Responses[0].SchemaRef != "#/components/schemas/ThingList" ||
		get.Responses[1].Status != "422" || get.Responses[1].SchemaRef != "#/components/schemas/ErrorEnvelope" {
		t.Fatalf("responses=%+v", get.Responses)
	}

	post := things.Endpoints[1]
	if post.Method != http.MethodPost || post.AuthLabel != "公开访问" {
		t.Fatalf("post endpoint=%+v", post)
	}
	if len(post.Responses) != 1 || post.Responses[0].Status != "204" || post.Responses[0].SchemaRef != "" {
		t.Fatalf("post responses=%+v", post.Responses)
	}

	var markdown legacyAPIDocsMarkdownData
	if err := json.Unmarshal([]byte(string(model.MarkdownJSON)), &markdown); err != nil {
		t.Fatal(err)
	}
	if len(markdown.Endpoints) != 3 || len(markdown.Groups) != 2 {
		t.Fatalf("markdown endpoints=%d groups=%d", len(markdown.Endpoints), len(markdown.Groups))
	}
	endpointMarkdown := markdown.Endpoints["get-api-things"]
	for _, fragment := range []string{
		"## GET /api/things",
		"事物列表",
		"- 认证：humanSession",
		"- Capability：`things.read`",
		"| `page` | query | 是 | integer | int64 | - | min=1 max=200 |",
		"| 422 | `#/components/schemas/ErrorEnvelope` |",
	} {
		if !strings.Contains(endpointMarkdown, fragment) {
			t.Fatalf("endpoint markdown missing %q:\n%s", fragment, endpointMarkdown)
		}
	}
	if !strings.Contains(markdown.Full, "# AI-CRM API 文档") || !strings.Contains(markdown.Groups["other"], "## POST /api/things") {
		t.Fatalf("markdown payload incomplete")
	}
}

func TestLegacyAPIDocsModelSortsEndpointsByPathThenFrozenMethodOrder(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /api/b:
    post:
      summary: b post
      responses:
        "204":
          description: ok
    get:
      summary: b get
      responses:
        "200":
          description: ok
  /api/a:
    delete:
      summary: a delete
      responses:
        "204":
          description: ok
    get:
      summary: a get
      responses:
        "200":
          description: ok
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/a", "DELETE /api/a", "GET /api/b", "POST /api/b"}
	if len(model.QuickReference) != len(want) {
		t.Fatalf("quick=%+v", model.QuickReference)
	}
	for index, entry := range model.QuickReference {
		if got := entry.Method + " " + entry.Path; got != want[index] {
			t.Fatalf("quick[%d]=%q, want %q (all=%+v)", index, got, want[index], model.QuickReference)
		}
	}
}

func TestLegacyAPIDocsModelFiltersNonDocumentedPaths(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /admin/api-docs:
    get:
      summary: 文档页自身
      responses:
        "200":
          description: ok
  /static/app.js:
    get:
      summary: 静态资源
      responses:
        "200":
          description: ok
  /internal/probe:
    get:
      summary: 内部探针
      responses:
        "200":
          description: ok
  /api/ok:
    get:
      summary: 公开接口
      responses:
        "200":
          description: ok
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	if model.EndpointCount != 1 || model.QuickReference[0].Path != "/api/ok" {
		t.Fatalf("quick=%+v", model.QuickReference)
	}
}

func TestLegacyAPIDocsModelSkipsHeadAndOptions(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /api/x:
    get:
      summary: get x
      responses:
        "200":
          description: ok
    head:
      summary: head x
      responses:
        "200":
          description: ok
    options:
      summary: options x
      responses:
        "204":
          description: ok
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	if model.EndpointCount != 1 || model.QuickReference[0].Method != http.MethodGet {
		t.Fatalf("quick=%+v", model.QuickReference)
	}
}

func TestLegacyAPIDocsModelDeduplicatesSharedAndOperationParameters(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /api/x:
    parameters:
      - name: page
        in: query
        required: false
        schema:
          type: integer
    get:
      summary: get x
      parameters:
        - name: page
          in: query
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: ok
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	endpoints := model.Groups[0].Endpoints
	if len(endpoints) != 1 || len(endpoints[0].Params) != 1 {
		t.Fatalf("params=%+v", endpoints[0].Params)
	}
	if endpoints[0].Params[0].Required {
		t.Fatalf("first shared declaration must win: %+v", endpoints[0].Params[0])
	}
}

func TestLegacyAPIDocsModelFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want string
	}{
		{"invalid yaml", "paths: [", ""},
		{"paths not mapping", "paths:\n  - item\n", "paths must be a mapping"},
		{"duplicate path", `paths:
  /api/x:
    get:
      summary: a
      responses:
        "200":
          description: ok
  /api/x:
    get:
      summary: b
      responses:
        "200":
          description: ok
`, "duplicate path"},
		{"duplicate method", `paths:
  /api/x:
    get:
      summary: a
      responses:
        "200":
          description: ok
    get:
      summary: b
      responses:
        "200":
          description: ok
`, "duplicate method"},
		{"duplicate operation after param type strip", `paths:
  /api/things/{id}:
    get:
      summary: a
      responses:
        "200":
          description: ok
  /api/things/{id:int}:
    get:
      summary: b
      responses:
        "200":
          description: ok
`, "duplicate operation"},
		{"anchor collision", `paths:
  /api/things-list:
    get:
      summary: a
      responses:
        "200":
          description: ok
  /api/things_list:
    get:
      summary: b
      responses:
        "200":
          description: ok
`, "anchor collision"},
		{"unsupported path item key", `paths:
  /api/x:
    trace:
      summary: a
      responses:
        "200":
          description: ok
`, "unsupported key"},
		{"unknown parameter reference", `paths:
  /api/x:
    get:
      summary: a
      parameters:
        - $ref: "#/components/parameters/Missing"
      responses:
        "200":
          description: ok
`, "unknown parameter reference"},
		{"chained parameter reference", `paths:
  /api/x:
    get:
      summary: a
      parameters:
        - $ref: "#/components/parameters/Chained"
      responses:
        "200":
          description: ok
components:
  parameters:
    Chained:
      $ref: "#/components/parameters/Other"
`, "chained parameter reference"},
		{"unknown response reference", `paths:
  /api/x:
    get:
      summary: a
      responses:
        "500":
          $ref: "#/components/responses/Missing"
`, "unknown response reference"},
		{"empty parameter name", `paths:
  /api/x:
    get:
      summary: a
      parameters:
        - in: query
          schema:
            type: string
      responses:
        "200":
          description: ok
`, "empty name"},
		{"unsupported parameter location", `paths:
  /api/x:
    get:
      summary: a
      parameters:
        - name: payload
          in: body
          schema:
            type: string
      responses:
        "200":
          description: ok
`, "unsupported location"},
		{"empty security scheme", `paths:
  /api/x:
    get:
      summary: a
      security:
        - "": []
      responses:
        "200":
          description: ok
`, "empty security scheme"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildLegacyAPIDocsModel([]byte(legacyAPIDocsTestSpecHead + test.spec))
			if err == nil {
				t.Fatal("expected construction to fail")
			}
			if !errors.Is(err, errLegacyAPIDocsInvalid) {
				t.Fatalf("error must wrap errLegacyAPIDocsInvalid: %v", err)
			}
			if test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q missing %q", err.Error(), test.want)
			}
		})
	}
}

func TestLegacyAPIDocsModelBuildsFromEmbeddedCanonicalSpec(t *testing.T) {
	model, err := buildLegacyAPIDocsModel(contractapi.OpenAPISpec())
	if err != nil {
		t.Fatal(err)
	}
	if model.EndpointCount < 40 || model.EndpointCount != len(model.QuickReference) {
		t.Fatalf("endpoint_count=%d quick=%d", model.EndpointCount, len(model.QuickReference))
	}
	if len(model.Groups) == 0 || len(model.Groups) > len(legacyAPIDocsGroupSpecs) {
		t.Fatalf("groups=%d", len(model.Groups))
	}

	// Group order must follow the frozen fifteen-group specification order.
	lastSpecIndex := -1
	for _, group := range model.Groups {
		specIndex := -1
		for index, spec := range legacyAPIDocsGroupSpecs {
			if spec.id == group.ID {
				specIndex = index
				break
			}
		}
		if specIndex == -1 || specIndex <= lastSpecIndex {
			t.Fatalf("group %q out of frozen order (index=%d after %d)", group.ID, specIndex, lastSpecIndex)
		}
		lastSpecIndex = specIndex
	}

	foundCustomers := false
	for _, group := range model.Groups {
		for _, endpoint := range group.Endpoints {
			if strings.HasPrefix(endpoint.Path, "/admin") || strings.HasPrefix(endpoint.Path, "/static") ||
				strings.Contains(endpoint.Path, "mcp-tools") {
				t.Fatalf("forbidden path documented: %+v", endpoint)
			}
			if endpoint.AuthLabel == "" || endpoint.Method == "" || endpoint.MethodLower != strings.ToLower(endpoint.Method) {
				t.Fatalf("endpoint missing safe fields: %+v", endpoint)
			}
			if endpoint.ID != endpoint.MethodLower+"-"+legacyAPIDocsSlug(endpoint.Path) {
				t.Fatalf("anchor %q does not match method+slug for %s %s", endpoint.ID, endpoint.Method, endpoint.Path)
			}
			if endpoint.Method == http.MethodGet && endpoint.Path == "/api/customers" {
				foundCustomers = true
				if group.ID != "customer-identity-sidebar" {
					t.Fatalf("GET /api/customers grouped into %q", group.ID)
				}
			}
		}
		// Endpoints inside a group stay sorted by path then frozen method order.
		for index := 1; index < len(group.Endpoints); index++ {
			left, right := group.Endpoints[index-1], group.Endpoints[index]
			if left.Path > right.Path ||
				(left.Path == right.Path && legacyAPIDocsMethodOrder[left.Method] >= legacyAPIDocsMethodOrder[right.Method]) {
				t.Fatalf("group %q endpoints out of order: %s %s before %s %s",
					group.ID, left.Method, left.Path, right.Method, right.Path)
			}
		}
	}
	if !foundCustomers {
		t.Fatal("GET /api/customers missing from embedded projection")
	}
	if model.MarkdownSizeLabel == "" {
		t.Fatal("markdown size label empty")
	}
	var markdown legacyAPIDocsMarkdownData
	if err := json.Unmarshal([]byte(string(model.MarkdownJSON)), &markdown); err != nil {
		t.Fatal(err)
	}
	if len(markdown.Endpoints) != model.EndpointCount || !strings.Contains(markdown.Full, "# AI-CRM API 文档") {
		t.Fatalf("markdown payload mismatch: endpoints=%d", len(markdown.Endpoints))
	}
}

// ---- page handler tests ----------------------------------------------------

func TestLegacyAPIDocsHandlerServesFrozenPageShell(t *testing.T) {
	handler, err := newLegacyAPIDocsHandler()
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, legacyApiDocsPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	headerChecks := map[string]string{
		"Content-Type":            "text/html; charset=utf-8",
		"Cache-Control":           "no-store",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": legacyApiDocsCSP,
	}
	for name, want := range headerChecks {
		if got := response.Header().Get(name); got != want {
			t.Fatalf("header %s=%q, want %q", name, got, want)
		}
	}
	for _, directive := range []string{"connect-src 'none'", "form-action 'none'", "base-uri 'none'"} {
		if !strings.Contains(legacyApiDocsCSP, directive) {
			t.Fatalf("CSP missing %q", directive)
		}
	}

	body := response.Body.String()
	for _, fragment := range []string{
		"<title>API 文档</title>",
		`id="api-docs-search"`,
		`id="apidoc-md-data"`,
		`data-copy-md="full"`,
		`data-copy-md="group:commerce"`,
		`data-copy-md="ep:get-api-customers"`,
		"快速索引",
		"个接口",
		`<code>/api/customers</code>`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("page missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"x-aicrm", "x-legacy", "x-p4", "operationId", "curl", "example", "Example",
		"<code>/admin/", "http://", "https://",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("page leaks forbidden content %q", forbidden)
		}
	}
}

func TestLegacyAPIDocsHandlerEscapesInjectedMarkup(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /api/x:
    get:
      summary: "<script>alert(1)</script>"
      description: "\" onmouseover=\"alert(2)"
      responses:
        "200":
          description: ok
  /api/y:
    get:
      summary: "</script><script>alert(3)</script>"
      responses:
        "200":
          description: ok
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	handler := &legacyAPIDocsHandler{model: model}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, legacyApiDocsPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	body := response.Body.String()
	for _, raw := range []string{
		"<script>alert(1)</script>",
		`onmouseover="alert(2)`,
		"</script><script>alert(3)</script>",
	} {
		if strings.Contains(body, raw) {
			t.Fatalf("page renders unsanitized content %q", raw)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("escaped summary missing")
	}
}

func TestLegacyAPIDocsHandlerTemplateFailureUsesCanonicalInternalError(t *testing.T) {
	spec := legacyAPIDocsTestSpecHead + `paths:
  /api/x:
    get:
      summary: a
      responses:
        "200":
          description: ok
`
	model, err := buildLegacyAPIDocsModel([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	original := legacyApiDocsTemplate
	legacyApiDocsTemplate = template.Must(template.New("legacy-api-docs-broken").Parse(`{{index .Groups 9999}}`))
	defer func() { legacyApiDocsTemplate = original }()

	handler := &legacyAPIDocsHandler{model: model}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, legacyApiDocsPath, nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("content-type=%q", contentType)
	}
	if strings.Contains(response.Body.String(), "legacy-api-docs") || strings.Contains(response.Body.String(), "index out of range") {
		t.Fatalf("internal error leaks template detail: %s", response.Body.String())
	}
	if !strings.HasPrefix(strings.TrimSpace(response.Body.String()), "{") {
		t.Fatalf("template failure must not emit partial HTML: %q", response.Body.String())
	}
}

// TestLegacyAPIDocsRenderSmokeFixture renders the real page into the file named
// by AICRM_API_DOCS_SMOKE_OUT so the headless browser smoke script can serve
// it. Without the environment variable the test skips.
func TestLegacyAPIDocsRenderSmokeFixture(t *testing.T) {
	out := os.Getenv("AICRM_API_DOCS_SMOKE_OUT")
	if out == "" {
		t.Skip("AICRM_API_DOCS_SMOKE_OUT not set")
	}
	handler, err := newLegacyAPIDocsHandler()
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, legacyApiDocsPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	if err := os.WriteFile(out, response.Body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---- redirect handler tests -------------------------------------------------

func TestLegacyMcpToolsRedirectIsExactAndDropsInput(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, legacyMcpToolsPath+"?next=https://evil.example&x=<script>", strings.NewReader("ignored-body"))
	response := httptest.NewRecorder()
	legacyMcpToolsRedirect(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("status=%d", response.Code)
	}
	if location := response.Header().Get("Location"); location != legacyApiDocsPath {
		t.Fatalf("location=%q", location)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("cache-control=%q", cacheControl)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body=%q", response.Body.String())
	}
}

// ---- registration-level black-box tests ------------------------------------

func legacyAPIDocsRouter(t *testing.T, service authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsAndMedia(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{})
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func legacyAPIDocsRequest(method, target string, withSession bool) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if withSession {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(63)})
	}
	return request
}

// assertCanonicalErrorEnvelope proves that authentication/authorization
// rejections use the canonical JSON error envelope, not an ad-hoc body.
func assertCanonicalErrorEnvelope(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("error content-type=%q", contentType)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("error body is not canonical JSON: %v", err)
	}
	for _, key := range []string{"code", "message", "request_id"} {
		if len(envelope[key]) == 0 {
			t.Fatalf("error envelope missing %q: %s", key, response.Body.String())
		}
	}
}

func TestLegacyAPIDocsPageRequiresSessionAndReadCapabilityWithoutCSRF(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		auth := &legacyMediaAuthStub{}
		router := legacyAPIDocsRouter(t, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyAPIDocsRequest(http.MethodGet, legacyApiDocsPath, false))
		if response.Code != http.StatusUnauthorized || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
			t.Fatalf("status=%d authenticate_calls=%d capabilities=%v csrf_calls=%d",
				response.Code, auth.authenticateCalls, auth.seen, auth.csrfCalls)
		}
		assertCanonicalErrorEnvelope(t, response)
	})

	t.Run("forbidden", func(t *testing.T) {
		auth := &legacyMediaAuthStub{authorizeErr: authport.ErrUnauthorized}
		router := legacyAPIDocsRouter(t, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyAPIDocsRequest(http.MethodGet, legacyApiDocsPath, true))
		if response.Code != http.StatusForbidden || auth.authenticateCalls != 1 || len(auth.seen) != 1 ||
			auth.seen[0] != authport.CapabilityConfigOverviewRead || auth.csrfCalls != 0 {
			t.Fatalf("status=%d authenticate_calls=%d capabilities=%v csrf_calls=%d",
				response.Code, auth.authenticateCalls, auth.seen, auth.csrfCalls)
		}
		assertCanonicalErrorEnvelope(t, response)
	})

	t.Run("ok", func(t *testing.T) {
		auth := &legacyMediaAuthStub{}
		router := legacyAPIDocsRouter(t, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyAPIDocsRequest(http.MethodGet, legacyApiDocsPath, true))
		if response.Code != http.StatusOK || auth.authenticateCalls != 1 || len(auth.seen) != 1 ||
			auth.seen[0] != authport.CapabilityConfigOverviewRead || auth.csrfCalls != 0 {
			t.Fatalf("status=%d authenticate_calls=%d capabilities=%v csrf_calls=%d",
				response.Code, auth.authenticateCalls, auth.seen, auth.csrfCalls)
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Fatalf("content-type=%q", contentType)
		}
		if !strings.Contains(response.Body.String(), "<title>API 文档</title>") {
			t.Fatal("page shell missing")
		}
	})
}

func TestLegacyAPIDocsPageMethodMismatchUsesRouter405BeforeAuth(t *testing.T) {
	for _, method := range []string{http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			auth := &legacyMediaAuthStub{}
			router := legacyAPIDocsRouter(t, auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyAPIDocsRequest(method, legacyApiDocsPath, true))
			if response.Code != http.StatusMethodNotAllowed || auth.authenticateCalls != 0 || len(auth.seen) != 0 || auth.csrfCalls != 0 {
				t.Fatalf("status=%d authenticate_calls=%d capabilities=%v csrf_calls=%d",
					response.Code, auth.authenticateCalls, auth.seen, auth.csrfCalls)
			}
			if allow := response.Header().Get("Allow"); allow != http.MethodGet {
				t.Fatalf("allow=%q, want exactly GET", allow)
			}
			if body := response.Body.String(); strings.Contains(body, "/api/") || strings.Contains(body, "API 文档") {
				t.Fatalf("405 body leaks endpoint metadata: %q", body)
			}
		})
	}
}

func TestLegacyMcpToolsRedirectRegistration(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		auth := &legacyMediaAuthStub{}
		router := legacyAPIDocsRouter(t, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyAPIDocsRequest(http.MethodGet, legacyMcpToolsPath, false))
		if response.Code != http.StatusUnauthorized || auth.authenticateCalls != 0 {
			t.Fatalf("status=%d authenticate_calls=%d", response.Code, auth.authenticateCalls)
		}
	})

	t.Run("authorized exact redirect with dropped query", func(t *testing.T) {
		auth := &legacyMediaAuthStub{}
		router := legacyAPIDocsRouter(t, auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyAPIDocsRequest(http.MethodGet, legacyMcpToolsPath+"?next=https://evil.example", true))
		if response.Code != http.StatusFound || response.Header().Get("Location") != legacyApiDocsPath ||
			response.Body.Len() != 0 || auth.csrfCalls != 0 ||
			len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityConfigOverviewRead {
			t.Fatalf("status=%d location=%q body=%q capabilities=%v csrf_calls=%d",
				response.Code, response.Header().Get("Location"), response.Body.String(), auth.seen, auth.csrfCalls)
		}
	})

	for _, method := range []string{http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method+" sees 405 before auth", func(t *testing.T) {
			auth := &legacyMediaAuthStub{}
			router := legacyAPIDocsRouter(t, auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyAPIDocsRequest(method, legacyMcpToolsPath, true))
			if response.Code != http.StatusMethodNotAllowed || auth.authenticateCalls != 0 || len(auth.seen) != 0 {
				t.Fatalf("status=%d authenticate_calls=%d capabilities=%v",
					response.Code, auth.authenticateCalls, auth.seen)
			}
			if allow := response.Header().Get("Allow"); allow != http.MethodGet {
				t.Fatalf("allow=%q, want exactly GET", allow)
			}
			if location := response.Header().Get("Location"); location != "" {
				t.Fatalf("405 must not carry a redirect location: %q", location)
			}
		})
	}
}

func TestLegacyMcpToolsSaveIsNotRegistered(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			auth := &legacyMediaAuthStub{}
			router := legacyAPIDocsRouter(t, auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyAPIDocsRequest(method, legacyMcpSavePath, true))
			if response.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

// ---- embedded contract immutability -----------------------------------------

// TestOpenAPISpecNeverExposesEmbeddedSlice is the permanent negative proof that
// callers cannot mutate the canonical embedded contract through the accessor.
func TestOpenAPISpecNeverExposesEmbeddedSlice(t *testing.T) {
	first := contractapi.OpenAPISpec()
	if len(first) == 0 {
		t.Fatal("embedded spec is empty")
	}
	canonical := string(contractapi.OpenAPISpec())
	first[0] = 'X'
	first[len(first)-1] = 'X'
	if got := string(contractapi.OpenAPISpec()); got != canonical {
		t.Fatal("caller mutation leaked into the embedded canonical contract")
	}
}

// ---- LEGACY-API-0034 static guards ------------------------------------------

func TestLegacyMcpToolsSaveStaysDeferredPostLaunch(t *testing.T) {
	if strings.Contains(string(contractapi.OpenAPISpec()), "mcp-tools/save") {
		t.Fatal("canonical OpenAPI contract must not declare /admin/config/mcp-tools/save")
	}
	encoded, err := os.ReadFile("../../docs/api-mapping.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(string(encoded), "\n") {
		if !strings.Contains(line, `"LEGACY-API-0034"`) {
			continue
		}
		found = true
		if !strings.Contains(line, `"disposition":"DEFERRED_POST_LAUNCH"`) {
			t.Fatalf("LEGACY-API-0034 disposition changed: %s", line)
		}
	}
	if !found {
		t.Fatal("LEGACY-API-0034 mapping row missing")
	}
}

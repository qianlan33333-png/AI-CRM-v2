// This file adapts the frozen legacy API documentation page and the legacy
// MCP tools entry redirect at the aicrm composition root. It owns no tables,
// no settings, no provider calls and no business state; its only data source
// is the compile-time embedded canonical api/openapi.yaml contract.
//
// Frozen contract (docs/evidence/p1/P4-API-DOCS-MCP-R2-FACTS.md):
//   - LEGACY-API-0003 GET /admin/api-docs renders the safe documentation
//     projection; LEGACY-API-0033 GET /admin/config/mcp-tools redirects
//     exactly to /admin/api-docs with an empty body.
//   - LEGACY-API-0034 POST /admin/config/mcp-tools/save stays
//     DEFERRED_POST_LAUNCH and must never be registered here.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	_ "embed"

	"gopkg.in/yaml.v3"

	contractapi "github.com/qianlan33333-png/AI-CRM-v2/api"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyApiDocsPath    = "/admin/api-docs"
	legacyMcpToolsPath   = "/admin/config/mcp-tools"
	legacyMcpSavePath    = "/admin/config/mcp-tools/save"
	legacyApiDocsTitle   = "API 文档"
	legacyApiDocsSummary = "查看 AI-CRM 后台和外部集成 API 文档。"
	legacyApiDocsActive  = "api.admin_api_docs"

	legacyApiDocsCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; " +
		"img-src 'none'; connect-src 'none'; font-src 'none'; object-src 'none'; media-src 'none'; " +
		"frame-src 'none'; worker-src 'none'; child-src 'none'; form-action 'none'; base-uri 'none'"
)

//go:embed templates/legacy_api_docs.html
var legacyApiDocsTemplateSource string

var legacyApiDocsTemplate = template.Must(template.New("legacy-api-docs").Parse(legacyApiDocsTemplateSource))

// ---- canonical OpenAPI parsing --------------------------------------------

type legacyAPIDocsSpec struct {
	Security   []map[string][]string `yaml:"security"`
	Paths      yaml.Node             `yaml:"paths"`
	Components struct {
		Parameters map[string]legacyAPIDocsParameter `yaml:"parameters"`
		Responses  map[string]legacyAPIDocsResponse  `yaml:"responses"`
	} `yaml:"components"`
}

type legacyAPIDocsParameter struct {
	Ref         string               `yaml:"$ref"`
	Name        string               `yaml:"name"`
	In          string               `yaml:"in"`
	Required    bool                 `yaml:"required"`
	Description string               `yaml:"description"`
	Schema      *legacyAPIDocsSchema `yaml:"schema"`
}

type legacyAPIDocsSchema struct {
	Ref     string   `yaml:"$ref"`
	Type    string   `yaml:"type"`
	Format  string   `yaml:"format"`
	Enum    []string `yaml:"enum"`
	Minimum *float64 `yaml:"minimum"`
	Maximum *float64 `yaml:"maximum"`
}

type legacyAPIDocsMediaType struct {
	Schema *legacyAPIDocsSchema `yaml:"schema"`
}

type legacyAPIDocsResponse struct {
	Ref         string                            `yaml:"$ref"`
	Description string                            `yaml:"description"`
	Content     map[string]legacyAPIDocsMediaType `yaml:"content"`
}

type legacyAPIDocsOperation struct {
	Summary     string                           `yaml:"summary"`
	Description string                           `yaml:"description"`
	Security    *[]map[string][]string           `yaml:"security"`
	Parameters  []legacyAPIDocsParameter         `yaml:"parameters"`
	Responses   map[string]legacyAPIDocsResponse `yaml:"responses"`
	Capability  string                           `yaml:"x-aicrm-capability"`
	CSRF        string                           `yaml:"x-aicrm-session-bound-csrf"`
	Effect      string                           `yaml:"x-aicrm-external-effect"`
}

// ---- view model ------------------------------------------------------------

type legacyAPIDocsEndpoint struct {
	ID          string
	Method      string
	MethodLower string
	Path        string
	Summary     string
	Description string
	AuthLabel   string
	Capability  string
	CSRF        string
	Effect      string
	Params      []legacyAPIDocsParamView
	Responses   []legacyAPIDocsResponseView
}

type legacyAPIDocsParamView struct {
	Name     string
	Location string
	Required bool
	Type     string
	Format   string
	Enum     string
	Range    string
}

type legacyAPIDocsResponseView struct {
	Status    string
	SchemaRef string
}

type legacyAPIDocsGroup struct {
	ID          string
	Title       string
	Description string
	Endpoints   []legacyAPIDocsEndpoint
}

type legacyAPIDocsQuickRef struct {
	Method     string
	Path       string
	Summary    string
	AuthLabel  string
	GroupTitle string
	Anchor     string
}

type legacyAPIDocsMarkdownData struct {
	Endpoints map[string]string `json:"endpoints"`
	Groups    map[string]string `json:"groups"`
	Full      string            `json:"full"`
}

type legacyAPIDocsPageModel struct {
	Title             string
	Summary           string
	ActiveEndpoint    string
	Groups            []legacyAPIDocsGroup
	QuickReference    []legacyAPIDocsQuickRef
	EndpointCount     int
	MarkdownSizeLabel string
	MarkdownJSON      template.JS // json.Marshal output with default HTML escaping
}

// ---- frozen legacy classification tables -----------------------------------

// legacyAPIDocsMethodOrder freezes GET → POST → PUT → PATCH → DELETE; unknown
// methods sort last. HEAD and OPTIONS never enter the documentation list.
var legacyAPIDocsMethodOrder = map[string]int{
	http.MethodGet: 0, http.MethodPost: 1, http.MethodPut: 2,
	http.MethodPatch: 3, http.MethodDelete: 4,
}

type legacyAPIDocsGroupSpec struct {
	id          string
	title       string
	description string
}

// The fifteen frozen business groups in their fixed relative order.
var legacyAPIDocsGroupSpecs = []legacyAPIDocsGroupSpec{
	{"system-mcp", "系统 / MCP", "健康检查、系统探针与 MCP 网关入口。"},
	{"auth-callback", "认证 / 回调", "后台登录、企业微信授权回调、企微事件与支付回调。"},
	{"customer-identity-sidebar", "客户 / 身份 / 侧边栏", "客户列表、客户档案、最近消息、身份解析和侧边栏上下文。"},
	{"channels", "渠道码中心", "渠道资产、渠道联系人、绑定关系、分享链接与欢迎素材。"},
	{"questionnaires", "问卷", "后台问卷管理、H5 问卷访问、提交结果和微信网页授权。"},
	{"user-ops", "用户运营 / 激活", "用户运营看板、批量发送、免打扰、发送记录和激活 webhook。"},
	{"automation", "自动化运营", "自动化转化配置、任务、工作流、成员、Agent 产物和执行记录。"},
	{"group-ops", "群运营计划", "群运营计划、群资产同步、节点编排、定时执行和 webhook。"},
	{"wecom-tags", "企微标签", "企微客户标签、标签组、同步和 live 标记接口。"},
	{"materials-send-content", "素材 / 发送内容", "图片、附件、小程序素材库，以及发送内容校验、预览和素材选择器。"},
	{"commerce", "交易 / 商品", "商品页、下单、订单查询、微信支付、支付宝与后台交易管理。"},
	{"ai-assist-compat", "AI 助手 / 兼容代理", "AI 助手契约、云编排、定时任务兼容代理与外部适配入口。"},
	{"push-center", "推送中心", "统一推送任务查询与执行明细。"},
	{"external-effects", "外部动作队列排障", "External Effect Queue 的排障查询、diagnostics、run-due 和测试 receiver API。"},
	{"other", "其他 API", "当前公开 API 合同清单中尚未归入固定业务分组的接口。"},
}

// legacyAPIDocsAllowed reports whether a path enters the documentation list.
// The first-level allow-pattern is frozen by the legacy page facts.
func legacyAPIDocsAllowed(path string) bool {
	if strings.HasPrefix(path, "/static") {
		return false
	}
	if path == "/admin" || strings.HasPrefix(path, "/admin/") {
		return false
	}
	switch {
	case path == "/health", path == "/mcp":
		return true
	case path == "/login", path == "/logout":
		return true
	}
	for _, prefix := range []string{"/api/", "/wecom/", "/auth/wecom/", "/p/", "/pay/", "/s/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// legacyAPIDocsGroupID freezes the first-match group classification order.
func legacyAPIDocsGroupID(path string) string {
	has := func(prefixes ...string) bool {
		for _, prefix := range prefixes {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
		return false
	}
	switch {
	case path == "/health" || path == "/api/system/health" || path == "/mcp" || has("/api/frontend-compat/"):
		return "system-mcp"
	case path == "/login" || path == "/logout" || has("/auth/wecom/", "/wecom/", "/api/wecom/events") ||
		strings.HasSuffix(path, "/notify") || strings.HasSuffix(path, "/return"):
		return "auth-callback"
	case has("/api/customers", "/api/messages/", "/api/sidebar/", "/api/identity/", "/api/admin/identity/") ||
		has("/api/admin/customers/", "/api/admin/customers/profile"):
		return "customer-identity-sidebar"
	case has("/api/admin/webhooks"):
		return "auth-callback"
	case has("/api/admin/exports"):
		return "system-mcp"
	case has("/api/admin/channels", "/api/admin/channel-welcome-materials"):
		return "channels"
	case has("/api/admin/questionnaires", "/api/h5/questionnaires", "/api/h5/wechat/oauth"):
		return "questionnaires"
	case has("/api/admin/user-ops", "/api/customer-automation/activation-webhook"):
		return "user-ops"
	case has("/api/admin/automation-conversion/group-ops", "/api/automation/group-ops"):
		return "group-ops"
	case has("/api/admin/wecom/tags", "/api/admin/wecom/tag-groups"):
		return "wecom-tags"
	case has("/api/admin/image-library", "/api/admin/attachment-library", "/api/admin/miniprogram-library",
		"/api/admin/send-content", "/api/admin/material-picker"):
		return "materials-send-content"
	case has("/api/admin/wechat-pay", "/api/admin/service-period-products", "/api/admin/alipay", "/api/admin/orders",
		"/api/admin/payments", "/api/admin/refunds", "/api/products", "/api/h5/service-period-products",
		"/p/", "/pay/", "/s/", "/api/checkout/", "/api/orders/", "/api/wechat-pay/", "/api/alipay/"):
		return "commerce"
	case has("/api/admin/ai-assist", "/api/admin/ai-audience", "/api/ai/audience", "/api/admin/cloud-orchestrator"):
		return "ai-assist-compat"
	case has("/api/admin/push-center"):
		return "push-center"
	case has("/api/admin/external-effects", "/api/external-effects"):
		return "external-effects"
	case has("/api/admin/automation-conversion"):
		return "automation"
	default:
		return "other"
	}
}

var errLegacyAPIDocsInvalid = errors.New("legacy api docs contract cannot be built safely")

// ---- builder ---------------------------------------------------------------

func legacyAPIDocsSlug(path string) string {
	lowered := strings.ToLower(strings.ReplaceAll(path, "_", "-"))
	lowered = strings.ReplaceAll(lowered, "{", "")
	lowered = strings.ReplaceAll(lowered, "}", "")
	var builder strings.Builder
	dash := false
	for _, r := range lowered {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			builder.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "endpoint"
	}
	return slug
}

func legacyAPIDocsStripParamTypes(path string) string {
	var builder strings.Builder
	inParam := false
	skip := false
	for _, r := range path {
		switch {
		case r == '{':
			inParam, skip = true, false
			builder.WriteRune(r)
		case r == '}':
			inParam, skip = false, false
			builder.WriteRune(r)
		case inParam && r == ':':
			skip = true
		case inParam && skip:
			// drop type suffix runes
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func legacyAPIDocsResolveParameter(ref string, components map[string]legacyAPIDocsParameter) (legacyAPIDocsParameter, error) {
	const prefix = "#/components/parameters/"
	if !strings.HasPrefix(ref, prefix) || len(ref) == len(prefix) {
		return legacyAPIDocsParameter{}, fmt.Errorf("%w: unsupported parameter reference %q", errLegacyAPIDocsInvalid, ref)
	}
	name := ref[len(prefix):]
	parameter, ok := components[name]
	if !ok {
		return legacyAPIDocsParameter{}, fmt.Errorf("%w: unknown parameter reference %q", errLegacyAPIDocsInvalid, ref)
	}
	if parameter.Ref != "" {
		return legacyAPIDocsParameter{}, fmt.Errorf("%w: chained parameter reference %q", errLegacyAPIDocsInvalid, ref)
	}
	return parameter, nil
}

func legacyAPIDocsResolveResponse(ref string, components map[string]legacyAPIDocsResponse) (legacyAPIDocsResponse, error) {
	const prefix = "#/components/responses/"
	if !strings.HasPrefix(ref, prefix) || len(ref) == len(prefix) {
		return legacyAPIDocsResponse{}, fmt.Errorf("%w: unsupported response reference %q", errLegacyAPIDocsInvalid, ref)
	}
	name := ref[len(prefix):]
	response, ok := components[name]
	if !ok {
		return legacyAPIDocsResponse{}, fmt.Errorf("%w: unknown response reference %q", errLegacyAPIDocsInvalid, ref)
	}
	if response.Ref != "" {
		return legacyAPIDocsResponse{}, fmt.Errorf("%w: chained response reference %q", errLegacyAPIDocsInvalid, ref)
	}
	return response, nil
}

func legacyAPIDocsAuthLabel(operation *legacyAPIDocsOperation, globalSecurity []map[string][]string) (string, error) {
	security := globalSecurity
	if operation.Security != nil {
		security = *operation.Security
	}
	if len(security) == 0 {
		return "公开访问", nil
	}
	schemes := make([]string, 0, len(security))
	seen := map[string]bool{}
	for _, requirement := range security {
		for scheme := range requirement {
			if scheme == "" {
				return "", fmt.Errorf("%w: empty security scheme", errLegacyAPIDocsInvalid)
			}
			if !seen[scheme] {
				seen[scheme] = true
				schemes = append(schemes, scheme)
			}
		}
	}
	sort.Strings(schemes)
	return strings.Join(schemes, " + "), nil
}

func legacyAPIDocsParamRange(schema *legacyAPIDocsSchema) string {
	if schema == nil {
		return ""
	}
	parts := []string{}
	if schema.Minimum != nil {
		parts = append(parts, fmt.Sprintf("min=%v", *schema.Minimum))
	}
	if schema.Maximum != nil {
		parts = append(parts, fmt.Sprintf("max=%v", *schema.Maximum))
	}
	return strings.Join(parts, " ")
}

func legacyAPIDocsParamViewOf(parameter legacyAPIDocsParameter) (legacyAPIDocsParamView, error) {
	if strings.TrimSpace(parameter.Name) == "" {
		return legacyAPIDocsParamView{}, fmt.Errorf("%w: parameter with empty name", errLegacyAPIDocsInvalid)
	}
	switch parameter.In {
	case "path", "query", "header", "cookie":
	default:
		return legacyAPIDocsParamView{}, fmt.Errorf("%w: parameter %q has unsupported location %q", errLegacyAPIDocsInvalid, parameter.Name, parameter.In)
	}
	view := legacyAPIDocsParamView{
		Name:     parameter.Name,
		Location: parameter.In,
		Required: parameter.Required,
	}
	if parameter.Schema != nil {
		view.Type = parameter.Schema.Type
		view.Format = parameter.Schema.Format
		if len(parameter.Schema.Enum) > 0 {
			view.Enum = strings.Join(parameter.Schema.Enum, " | ")
		}
		view.Range = legacyAPIDocsParamRange(parameter.Schema)
	}
	return view, nil
}

func legacyAPIDocsResponseViews(operation *legacyAPIDocsOperation, components map[string]legacyAPIDocsResponse) ([]legacyAPIDocsResponseView, error) {
	statuses := make([]string, 0, len(operation.Responses))
	for status := range operation.Responses {
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return legacyAPIDocsStatusLess(statuses[i], statuses[j])
	})
	views := make([]legacyAPIDocsResponseView, 0, len(statuses))
	for _, status := range statuses {
		response := operation.Responses[status]
		if response.Ref != "" {
			resolved, err := legacyAPIDocsResolveResponse(response.Ref, components)
			if err != nil {
				return nil, err
			}
			response = resolved
		}
		schemaRef := ""
		mediaTypes := make([]string, 0, len(response.Content))
		for mediaType := range response.Content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			if schema := response.Content[mediaType].Schema; schema != nil && schema.Ref != "" {
				schemaRef = schema.Ref
				break
			}
		}
		views = append(views, legacyAPIDocsResponseView{Status: status, SchemaRef: schemaRef})
	}
	return views, nil
}

func legacyAPIDocsStatusLess(left, right string) bool {
	if left == "default" {
		return right != "default"
	}
	if right == "default" {
		return true
	}
	return left < right
}

// buildLegacyAPIDocsModel parses the embedded canonical OpenAPI contract once
// and derives the immutable, sanitized page view model. It is fail-closed:
// parse errors, duplicate (method,path) pairs, anchor collisions and unknown
// structure all abort construction.
func buildLegacyAPIDocsModel(spec []byte) (*legacyAPIDocsPageModel, error) {
	var document legacyAPIDocsSpec
	if err := yaml.Unmarshal(spec, &document); err != nil {
		return nil, fmt.Errorf("%w: %v", errLegacyAPIDocsInvalid, err)
	}
	if document.Paths.Kind != yaml.MappingNode || len(document.Paths.Content)%2 != 0 {
		return nil, fmt.Errorf("%w: paths must be a mapping", errLegacyAPIDocsInvalid)
	}

	seenPaths := map[string]bool{}
	seenOperations := map[string]bool{}
	seenAnchors := map[string]bool{}
	endpoints := []legacyAPIDocsEndpoint{}

	pathItems := document.Paths.Content
	for index := 0; index < len(pathItems); index += 2 {
		rawPath := pathItems[index].Value
		itemNode := pathItems[index+1]
		if seenPaths[rawPath] {
			return nil, fmt.Errorf("%w: duplicate path %q", errLegacyAPIDocsInvalid, rawPath)
		}
		seenPaths[rawPath] = true
		if itemNode.Kind != yaml.MappingNode || len(itemNode.Content)%2 != 0 {
			return nil, fmt.Errorf("%w: path item %q must be a mapping", errLegacyAPIDocsInvalid, rawPath)
		}

		sharedParams := []legacyAPIDocsParameter{}
		methodNodes := map[string]*yaml.Node{}
		for field := 0; field < len(itemNode.Content); field += 2 {
			key := strings.ToLower(itemNode.Content[field].Value)
			value := itemNode.Content[field+1]
			switch key {
			case "parameters":
				if err := value.Decode(&sharedParams); err != nil {
					return nil, fmt.Errorf("%w: shared parameters of %q: %v", errLegacyAPIDocsInvalid, rawPath, err)
				}
			case "get", "post", "put", "patch", "delete":
				if methodNodes[key] != nil {
					return nil, fmt.Errorf("%w: duplicate method %s %q", errLegacyAPIDocsInvalid, strings.ToUpper(key), rawPath)
				}
				methodNodes[key] = value
			case "head", "options":
				// HEAD and OPTIONS never enter the documentation list.
			default:
				return nil, fmt.Errorf("%w: path item %q has unsupported key %q", errLegacyAPIDocsInvalid, rawPath, itemNode.Content[field].Value)
			}
		}

		path := legacyAPIDocsStripParamTypes(rawPath)
		if path == "" || !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("%w: invalid path %q", errLegacyAPIDocsInvalid, rawPath)
		}
		if !legacyAPIDocsAllowed(path) {
			continue
		}

		for method, node := range methodNodes {
			operationKey := strings.ToUpper(method) + " " + path
			if seenOperations[operationKey] {
				return nil, fmt.Errorf("%w: duplicate operation %q", errLegacyAPIDocsInvalid, operationKey)
			}
			seenOperations[operationKey] = true

			var operation legacyAPIDocsOperation
			if err := node.Decode(&operation); err != nil {
				return nil, fmt.Errorf("%w: operation %q: %v", errLegacyAPIDocsInvalid, operationKey, err)
			}

			authLabel, err := legacyAPIDocsAuthLabel(&operation, document.Security)
			if err != nil {
				return nil, fmt.Errorf("%w: operation %q: %v", errLegacyAPIDocsInvalid, operationKey, err)
			}

			params := append(append([]legacyAPIDocsParameter{}, sharedParams...), operation.Parameters...)
			paramViews := make([]legacyAPIDocsParamView, 0, len(params))
			seenParams := map[string]bool{}
			for _, parameter := range params {
				if parameter.Ref != "" {
					resolved, err := legacyAPIDocsResolveParameter(parameter.Ref, document.Components.Parameters)
					if err != nil {
						return nil, fmt.Errorf("%w: operation %q: %v", errLegacyAPIDocsInvalid, operationKey, err)
					}
					parameter = resolved
				}
				view, err := legacyAPIDocsParamViewOf(parameter)
				if err != nil {
					return nil, fmt.Errorf("%w: operation %q: %v", errLegacyAPIDocsInvalid, operationKey, err)
				}
				paramKey := view.Location + "\x00" + view.Name
				if seenParams[paramKey] {
					continue
				}
				seenParams[paramKey] = true
				paramViews = append(paramViews, view)
			}

			responseViews, err := legacyAPIDocsResponseViews(&operation, document.Components.Responses)
			if err != nil {
				return nil, fmt.Errorf("%w: operation %q: %v", errLegacyAPIDocsInvalid, operationKey, err)
			}

			methodUpper := strings.ToUpper(method)
			anchor := strings.ToLower(method) + "-" + legacyAPIDocsSlug(path)
			if !seenAnchors[anchor] {
				seenAnchors[anchor] = true
			} else {
				return nil, fmt.Errorf("%w: anchor collision %q", errLegacyAPIDocsInvalid, anchor)
			}

			endpoints = append(endpoints, legacyAPIDocsEndpoint{
				ID:          anchor,
				Method:      methodUpper,
				MethodLower: method,
				Path:        path,
				Summary:     strings.TrimSpace(operation.Summary),
				Description: strings.TrimSpace(operation.Description),
				AuthLabel:   authLabel,
				Capability:  operation.Capability,
				CSRF:        operation.CSRF,
				Effect:      operation.Effect,
				Params:      paramViews,
				Responses:   responseViews,
			})
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return legacyAPIDocsMethodOrder[endpoints[i].Method] < legacyAPIDocsMethodOrder[endpoints[j].Method]
	})

	grouped := map[string][]legacyAPIDocsEndpoint{}
	for _, endpoint := range endpoints {
		groupID := legacyAPIDocsGroupID(endpoint.Path)
		grouped[groupID] = append(grouped[groupID], endpoint)
	}

	groups := make([]legacyAPIDocsGroup, 0, len(legacyAPIDocsGroupSpecs))
	quick := make([]legacyAPIDocsQuickRef, 0, len(endpoints))
	for _, spec := range legacyAPIDocsGroupSpecs {
		groupEndpoints := grouped[spec.id]
		if len(groupEndpoints) == 0 {
			continue
		}
		groups = append(groups, legacyAPIDocsGroup{
			ID: spec.id, Title: spec.title, Description: spec.description, Endpoints: groupEndpoints,
		})
		for _, endpoint := range groupEndpoints {
			quick = append(quick, legacyAPIDocsQuickRef{
				Method: endpoint.Method, Path: endpoint.Path, Summary: endpoint.Summary,
				AuthLabel: endpoint.AuthLabel, GroupTitle: spec.title, Anchor: endpoint.ID,
			})
		}
	}

	markdown := buildLegacyAPIDocsMarkdown(groups)
	// json.Marshal escapes <, > and & by default, so the payload cannot break
	// out of the application/json script element.
	markdownJSON, err := json.Marshal(markdown)
	if err != nil {
		return nil, fmt.Errorf("%w: markdown payload: %v", errLegacyAPIDocsInvalid, err)
	}

	return &legacyAPIDocsPageModel{
		Title:             legacyApiDocsTitle,
		Summary:           legacyApiDocsSummary,
		ActiveEndpoint:    legacyApiDocsActive,
		Groups:            groups,
		QuickReference:    quick,
		EndpointCount:     len(quick),
		MarkdownSizeLabel: legacyAPIDocsSizeLabel(len(markdown.Full)),
		MarkdownJSON:      template.JS(markdownJSON),
	}, nil
}

func legacyAPIDocsSizeLabel(bytes int) string {
	const mebibyte = 1024 * 1024
	if bytes >= mebibyte {
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mebibyte))
	}
	kb := (bytes + 512) / 1024
	if kb < 1 {
		kb = 1
	}
	return fmt.Sprintf("%d KB", kb)
}

// ---- markdown --------------------------------------------------------------

func legacyAPIDocsEndpointMarkdown(endpoint legacyAPIDocsEndpoint) string {
	var builder strings.Builder
	builder.WriteString("## " + endpoint.Method + " " + endpoint.Path + "\n\n")
	if endpoint.Summary != "" {
		builder.WriteString(endpoint.Summary + "\n\n")
	}
	if endpoint.Description != "" {
		builder.WriteString(endpoint.Description + "\n\n")
	}
	builder.WriteString("- 认证：" + endpoint.AuthLabel + "\n")
	if endpoint.Capability != "" {
		builder.WriteString("- Capability：`" + endpoint.Capability + "`\n")
	}
	if endpoint.CSRF != "" {
		builder.WriteString("- CSRF：" + endpoint.CSRF + "\n")
	}
	if endpoint.Effect != "" {
		builder.WriteString("- 外部效果：" + endpoint.Effect + "\n")
	}
	builder.WriteString("\n")
	if len(endpoint.Params) > 0 {
		builder.WriteString("| 参数 | 位置 | 必填 | 类型 | 格式 | 枚举 | 范围 |\n")
		builder.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, param := range endpoint.Params {
			required := "否"
			if param.Required {
				required = "是"
			}
			builder.WriteString("| `" + param.Name + "` | " + param.Location + " | " + required + " | " +
				legacyAPIDocsMarkdownCell(param.Type) + " | " + legacyAPIDocsMarkdownCell(param.Format) + " | " +
				legacyAPIDocsMarkdownCell(param.Enum) + " | " + legacyAPIDocsMarkdownCell(param.Range) + " |\n")
		}
		builder.WriteString("\n")
	}
	if len(endpoint.Responses) > 0 {
		builder.WriteString("| 状态 | Schema |\n| --- | --- |\n")
		for _, response := range endpoint.Responses {
			schema := response.SchemaRef
			if schema != "" {
				schema = "`" + schema + "`"
			}
			builder.WriteString("| " + response.Status + " | " + schema + " |\n")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func legacyAPIDocsMarkdownCell(value string) string {
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}

func buildLegacyAPIDocsMarkdown(groups []legacyAPIDocsGroup) legacyAPIDocsMarkdownData {
	data := legacyAPIDocsMarkdownData{
		Endpoints: map[string]string{},
		Groups:    map[string]string{},
	}
	var full strings.Builder
	full.WriteString("# AI-CRM API 文档\n\n")
	full.WriteString("本文档由服务端从仓库 canonical OpenAPI 合同生成，仅展示安全字段。\n\n")
	for _, group := range groups {
		var groupMarkdown strings.Builder
		groupMarkdown.WriteString("# " + group.Title + "\n\n" + group.Description + "\n\n")
		for _, endpoint := range group.Endpoints {
			endpointMarkdown := legacyAPIDocsEndpointMarkdown(endpoint)
			data.Endpoints[endpoint.ID] = endpointMarkdown
			groupMarkdown.WriteString(endpointMarkdown)
		}
		data.Groups[group.ID] = groupMarkdown.String()
		full.WriteString(groupMarkdown.String())
	}
	data.Full = full.String()
	return data
}

// ---- handlers --------------------------------------------------------------

type legacyAPIDocsHandler struct {
	model *legacyAPIDocsPageModel
}

// newLegacyAPIDocsHandler parses the embedded canonical contract and prepares
// the immutable page model. Any failure aborts construction, which fails the
// composition-root startup wiring.
func newLegacyAPIDocsHandler() (*legacyAPIDocsHandler, error) {
	model, err := buildLegacyAPIDocsModel(contractapi.OpenAPISpec())
	if err != nil {
		return nil, err
	}
	return &legacyAPIDocsHandler{model: model}, nil
}

func (handler *legacyAPIDocsHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Render into a buffer first: a template failure must never leave a
	// partially written 200 page that cannot be replaced by the canonical 500.
	var rendered bytes.Buffer
	if err := legacyApiDocsTemplate.Execute(&rendered, handler.model); err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeInternal, nil))
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("Content-Security-Policy", legacyApiDocsCSP)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(rendered.Bytes())
}

// legacyMcpToolsRedirect preserves LEGACY-API-0033: an authorized browser is
// redirected exactly to /admin/api-docs. The original query and body are never
// parsed, logged or echoed, and the response body stays empty.
func legacyMcpToolsRedirect(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Location", legacyApiDocsPath)
	writer.WriteHeader(http.StatusFound)
}

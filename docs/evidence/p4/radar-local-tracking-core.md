# P4 Radar Local Tracking Core 后端收据

## 边界

本包在 `00081_radar_local_tracking.sql` 上恢复 Radar 的本地后端追踪闭环，不包含前端、
PDF 上传/处理、OAuth、Provider 或真实客户身份归因。

- `GET /r/{code}` 仅解析 enabled Radar link，在同一 UoW 写入 `landing` 与 `redirect`
  两条不可变本地 receipt 后返回 302。
- `POST /api/h5/radar-contents/{code}/events` 仅接受旧协议已证明的 8 个 stage：
  `viewer_open`、`image_loaded`、`pdf_opened`、`pdf_manifest_loaded`、
  `pdf_page_loaded`、`pdf_page_error`、`image_manifest_loaded`、
  `image_variant_loaded`。写入要求 Idempotency-Key；只持久化 key/payload digest。
- 管理端提供 events 分页、stage/时间过滤、stats 和 CSV export。CSV 保留旧头
  `unionid,external_userid,created_at`，但前两列固定为空，因为本包不执行 OAuth/身份归因。
- 应用层提供 enabled-only sidebar 安全投影，仅包含
  `id,title,target_type,type_label,url,updated_at`；未在缺少 canonical public base URL
  合同的情况下猜测或新增 Sidebar HTTP 鉴权协议。

`radar_link_events` 不包含 IP、User-Agent、Referer、query、openid、unionid、
external_userid、customer_id 或 raw extra。所有 receipt 的
`identity_attributed=false`、`real_external_call_executed=false`。

## 旧协议与 Matrix 分层

OpenAPI 已绑定 `LEGACY-API-0457`、`LEGACY-API-0458`、`LEGACY-API-0462`、
`LEGACY-API-0654`、`LEGACY-API-0770`。协议来自
`docs/api-mapping.jsonl` 与 immutable legacy SHA
`6cb989c071255437d75953dabb943318a74eb8f4` 的
`aicrm_next/extensions/radar/radar_links/api.py` / `application.py`。

Matrix `LEGACY-S07-130`、`LEGACY-S07-139`、`LEGACY-S07-140` 的后端依赖已由本包
提供，但这些行仍包含前端弹窗、重绘或浏览器导航结果；按后端优先约束，不把缺失的前端
接入写成 Matrix 整体完成。PDF 与 OAuth 路由维持原 disposition，不由本包扩展。

## 验收

- `go test -race -count=1 ./internal/radar/... ./acceptance/radar ./cmd/aicrm`
- `make p4-radar-local-tracking-acceptance`
- `go -C tools test ./openapi-contract && go -C tools run ./openapi-contract`
- selected CI：`radar` Go/database group；Nightly manifest：`p4-radar-local-tracking`。

PG16.14 acceptance 在独立数据库验证 00081 fresh up、空表 down/up、8 路并发同 key
单 receipt、exact replay、payload conflict、过滤/统计/sidebar 投影、PII 最小化、
UPDATE guard 与 populated down guard。它只证明本地后端能力，不证明 main 合并、Nightly、
部署或真实外部效果。

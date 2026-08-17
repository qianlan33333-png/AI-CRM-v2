# P4-IMAGE-FACETS-0358

## 目标

从 exact-green main `90177c280021e7d0b6ea033e7e3203fa5a48620c` 恢复单一完整 API 行为：
`GET /api/admin/image-library/facets`（`LEGACY-API-0358`）。

## 冻结合同

- owner：Media；单实例 PostgreSQL `media_images.category/tags` 只读投影。
- 身份与权限：human session + `media.library.read`；GET 不要求 CSRF。
- success：严格九字段，`categories` / `tags` 始终为非 null 数组。
- category：`strings.TrimSpace`、丢弃空值、大小写敏感去重、Go 字符串排序，不做读取截断。
- tag：ASCII 逗号分割、`TrimSpace`、64 Unicode code point 截断，并保留每行最多 50 项的历史 quirk。
- error：复用 canonical 401/403/500；非 GET 由 Chi 在认证前返回 405。
- 无 OneID、tenant、migration、UoW 业务写、event、receipt、provider、URL/blob 读取或外部效果。

## 完整分母

- A：`LEGACY-API-0358` 一条 API 映射。
- B：零条可独立关闭的 Feature Matrix 行；`LEGACY-S07-049` 同时包含图片列表 API 与页面流，仍保持 `NOT_STARTED`，禁止以本 API 子集冒充整行完成。
- migration：零。
- migration closure：`no_schema_or_external_effect`；本片没有新增 schema、migration 或外部效果。
- deferred：`0359`、`0360`、`0365` 不在本片；既有 `0361` 不重复计数。

## 验收

- application、router/handler、真实 PostgreSQL repository 路径均须验证。
- generator 连续运行无 diff；generated/source manifest、API Mapping、acceptance manifest 与 repo fingerprints 同步。
- 必跑：focused + race、`go test ./...`、Web lint/type/test/build/audit、repo-contract、secret scan 和 protected-main 四项 Required Checks。
- staging、production、真实企微/支付/外发均 `NOT_EXECUTED`。

# P4-IMAGE-LIST-0356
## 目标与冻结基线
从 exact-main `b73aa59276cdb4dc6075ca87fdf3c97f40cd9cf3`、tree
`0a2848b927a2a09e490689896e16fcb9234f78e7` 恢复一个完整只读 API 行为：
`LEGACY-API-0356 GET /api/admin/image-library`。

本片只关闭 0356 的候选实现，不包含页面、创建、详情、更新、删除、旧 thumbnail
或 variant serving。正式集成必须与独立 denominator `LEGACY-API-0366` 的可用性依赖
一并排期；本片只投影冻结的相对 variant URL，不生成、不读取也不验证变体。

## 冻结 HTTP 合同
- audience：后台 human session；capability：`media.library.read`；admin/ops global。
- GET 不要求 CSRF；不 redirect；未独立注册的方法由 Chi 在认证前返回 405。
- 仅允许八类 query：`limit`、`offset`、`enabled_only`、`q`、`category`、
  `tags`、可重复 `tag_group`、`only_unlabeled`；未知 key、malformed percent/UTF-8
  返回 canonical 422。
- 除 `tag_group` 外，每个参数存在时必须恰好一个 scalar。
- `limit` / `offset` 词法必须匹配 `^-?[0-9]+$`，空白、空值、小数、指数、
  `+`、重复和 int64 溢出均 422；解析后沿用 legacy clamp：
  `limit=0/omitted -> 100`、负数 -> 1、上限 500；负 offset -> 0。
- bool 只接受小写 `true` / `false`；空值、大小写变体和重复均 422。
- 422/401/403/500 使用 canonical `ErrorResponse`，不得复制 FastAPI `detail`，
  不得泄漏 SQL、actor、storage 或 legacy error envelope。

## 筛选、排序与分页
筛选代数固定为 `Q AND C AND T AND G1 ... AND Gn AND U`：

- `q`：`TrimSpace` 后，以 PostgreSQL `ILIKE` 搜索 name、file_name、description、
  category 和每个 normalized tag；`%` / `_` 保留 wildcard 含义。
- `category`：query `TrimSpace` 后大小写敏感精确匹配。
- `tags`：ASCII comma split、Trim、drop empty、case-sensitive exact dedupe，
  再按 64 Unicode code point 截断、每行最多 50 项；保留 65+ 长值在截断前
  dedupe 所造成的重复输出 quirk；成员之间 OR。
- `tag_group`：每个值按相同规则归一化，空组丢弃；完整数组 exact dedupe，
  顺序参与 dedupe；组内 OR、组间 AND。
- `only_unlabeled=true`：description 空、category 空或 normalized tags 为空三者任一。
- `enabled_only`：批准的 no-schema compatibility no-op；所有 v2 image 永久 enabled，
  true/false 返回同一全集，item `enabled` 恒 true。
- 排序：`updated_at DESC, id DESC`。
- PostgreSQL repository 在一个 read UoW 的单个 SQL statement 中同时计算 total 和 page；
  `total` 是分页前总数，`count=len(items)`，`has_more=(offset+count)<total`，
  有更多时 `next_offset=offset+count`，否则为 null。

## 成功 DTO
顶层必须恰好 14 keys：

`ok,items,total,limit,offset,count,has_more,next_offset,source_status,route_owner,`
`fallback_used,real_external_call_executed,storage_adapter_mode,adapter_mode`。

固定值：`source_status=next_media_library`、`route_owner=ai_crm_next`、
`fallback_used=false`、`real_external_call_executed=false`、
`storage_adapter_mode=postgresql`、`adapter_mode=postgresql`。

每个 item 必须恰好 20 keys：

`id,name,file_name,mime_type,file_size,enabled,description,tags,category,width,height,`
`created_at,updated_at,thumb_160_url,thumb_320_url,thumb_url,preview_url,`
`mobile_1080_url,large_1440_url,original_url`。

- `created_at` / `updated_at`：非空 RFC3339 date-time string。
- `tags`：始终非 null string array。
- provider/cache/base64/blob/checksum/private/raw URL、source/source_url/content_type、
  `ai_metadata`、actor/created_by 均 absent。
- URL 只允许 `/api/admin/image-library/{id}/variants/` 下的同源相对字符串：
  `thumb_160`、`thumb_320`、`mobile_1080`、`large_1440`、`original`；其中
  `thumb_url=thumb_320_url`，`preview_url=mobile_1080_url`。

## Owner、事务与外效
- owner：Media；只读 `media_images` read projection。
- one read UoW；无业务写、event、receipt、outbox、River job 或幂等记录。
- 不读取 `media_image_blobs`，不调用 Provider、BlobStore、URL fetch、worker、企微、
  支付、外发或 thumbnail/variant generator。
- 无 OneID、tenant、跨域数据或新身份模型。
- `media_images` 现有 00030 schema 足够：`NO_SCHEMA_CHANGE`；不得新增 enabled 列、
  migration、table 或 index。

## 完整 A+B 分母
| Mapping / Feature | 本片处理 |
|---|---|
| `LEGACY-API-0052` page | 保持 pending；不实现、不计数 |
| `LEGACY-API-0356` GET list | 本片唯一候选 mapping；候选计数 1 |
| `LEGACY-API-0357` POST collection | 保留独立注册空间；不实现、不计数 |
| `LEGACY-API-0358` facets | 既有独立实现；不重复计数 |
| `LEGACY-API-0359` from-base64 | 保持 `DEFERRED_POST_LAUNCH` |
| `LEGACY-API-0360` from-url | 保持 `DEFERRED_POST_LAUNCH` |
| `LEGACY-API-0361` upload | 既有独立实现；不重复计数 |
| `LEGACY-API-0362` delete | pending；不实现、不计数 |
| `LEGACY-API-0363` detail | pending；不实现、不计数 |
| `LEGACY-API-0364` update | pending；不实现、不计数 |
| `LEGACY-API-0365` legacy thumbnail | 保持 `DEFERRED_POST_LAUNCH` |
| `LEGACY-API-0366` variants | pending；独立可用性依赖；不实现、不计数、不改状态 |
| `LEGACY-S07-017` page | 保持 `NOT_STARTED / NOT_RUN` |
| `LEGACY-S07-049` list + facets + UI flow | 保持 `NOT_STARTED / NOT_RUN`；不能由 API 子集 CLOSED |
| `LEGACY-S07-050` upload | 既有 0361；不重复计数 |

## 候选文件范围
仅允许本片 facts §9.6 的 12 路径：OpenAPI、API registration、新 list adapter、
既有 media HTTP test、Media app/list tests、Media port、list SQL/repository、sqlc generated、
API Mapping 和本 slice 卡。明确排除 migration、Feature Matrix CLOSED、0357–0366 route、
旧 Python/JS、workflow/checker、root dependency 和任何外部 adapter。

`internal/media/store/generated/**` 只能由仓库锁定的 sqlc 生成；禁止手改。完整仓库集成时
必须运行 sqlc，并验证连续第二次生成无 diff。

## Required acceptance
- focused application tests：clamp、tag/group normalization、enabled no-op、projection、
  exact URLs、one-UoW combined read、malformed rows、repository/UoW failure、zero write。
- focused HTTP tests：401、403、admin/ops global、GET/no-CSRF、405-before-auth、
  defaults、accepted query、所有 strict 422、exact 14/20 keys、non-null arrays、
  RFC3339、URL alias、forbidden-field absence、safe 500、malformed success fail-closed。
- PostgreSQL repository：单 statement same-snapshot total+page、empty/beyond-end、scan/query/rows
  failure、stable ordering、filters and combinations；真实 PostgreSQL integration required。
- generator：sqlc generation and clean second generation。
- full repository：focused/race、`go test ./...`、OpenAPI/repo-contract/source-policy、secret scan、
  required CI checks。
- staging、production、数据库写入/migration、Provider、企微、支付、部署和真实外发：
  `NOT_EXECUTED`，且不属于本片候选生成权限。

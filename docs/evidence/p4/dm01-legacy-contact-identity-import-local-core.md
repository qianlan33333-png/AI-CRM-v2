# DM01 Legacy Contact / Identity Import Local Core：V2 后端能力账本

## 能力边界

DM01 是 operator-controlled 的双库历史导入能力，只读固定 legacy
source，将 Contact / Identity 必要本地事实写入 V2 target。命令行显式提供
`preflight` / `full` / `incremental` / `reconcile` 四种 mode；每次运行绑定
manifest digest、legacy repository SHA、source snapshot、HMAC key version 和唯一
WeCom corp。

本包不启动普通 event/job，不触发 customer merge、pending event、Provider、企微、
支付、退款或其他外部效果，也不包含前端页面。本地 receipt 仅证明对
V2 数据库的事务提交与重放边界，不证明部署、切流或真实外部成功。

## 11 张源表处置

| 处置 | legacy table | V2 结果 |
| --- | --- | --- |
| active import | `owner_role_map` | 只在 corp-scoped owner allowlist 内导入 Staff 根事实 |
| active import | `crm_user_identity` | 以非空 `unionid` 导入 Customer 根事实；不把 legacy `mobile_verified` 当成 V2 已验证证据 |
| scoped bind | `wecom_external_contact_identity_map` | 仅绑定同 corp 且已唯一解析的 Customer / Staff；不猜测、不自动 merge |
| encrypted inactive archive | `crm_user_identity_merge_audit`, `crm_user_identity_resolution_queue` | AES-GCM 保留不活跃历史；不恢复 merge 或 pending work |
| deferred quarantine | `crm_user_identity_conflicts`, `people`, `wecom_external_contact_follow_users` | 仅保留 HMAC / reason / receipt，等待后续 target schema；不写 active runtime |
| source-readonly DROP / REBUILD disposition | `contacts`, `admin_wecom_directory_members`, `external_contact_bindings` | 扫描并出具 skipped receipt；不删 source，不建 V2 target，不重放旧行为 |

source 必须与 manifest 的 physical server / database / dedicated read-only role 一致，
同时命中运行时 allowlist；与 target 指向同一 physical database 时 fail-closed。
所有 canonical key 拒绝空值和前后空白，所有带 corp 的 projection 必须等于
manifest corp。

## 事务、重放与对账

- source 在 read-only repeatable-read snapshot 中固定 11 表 upper bound；每 500 行分页并续租。
- target 通过 lease generation / token CAS 拒绝过期 worker。每个 target write、mapping、
  quarantine / archive 与最后的 row receipt 在同一 UoW，receipt 是最后的 fenced write。
- 同 key / 同 payload 精确重放；同 key / 不同 payload 拒绝覆盖。incremental 仅在
  target 仍等于先前 digest 时 CAS 更新，否则 quarantine。
- reconcile 依 source 顺序重算 digest、处置数量和 companion，并对历史 archive
  重新解密验证 AAD / HMAC；对账只报错，不修复、不产生外部事实。
- migration `00072` 保留 run/checkpoint/mapping/receipt/quarantine/archive 证据；
  已完成或已产生事实时 down fail-closed。

## 验证口径

DM01 selected database gate 要求两个独立 PostgreSQL 16 数据库，同时执行 migration
up/down safety 和真实 executor E2E。验收覆盖同库拒绝、schema / mode drift 零目标写、
fixed upper bound、exact replay、crash resume、过期 fence 整页回滚、target drift
quarantine、archive 解密、reconcile tamper fail-closed，以及普通 event/job/provider
全程零产生。

本文档记录的是 DM01 本地后端合同。required CI、main 合并、exact-main Nightly、
部署、真实数据迁移与外部效果必须在各自层级单独报告，不因本文档或
本地测试自动升级为完成。

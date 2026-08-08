# AI-CRM v2 重构详细设计

> 版本 v1.0 · 基于 2026-08 需求确认
> 行为基准：当前线上 aicrm_next 实际行为
> 本文档是后续所有 AI 辅助开发任务（Claude Code / Codex）的唯一架构口径

---

## 0. 定稿约束（不再讨论）

| 项 | 结论 |
|---|---|
| 后端语言 | Go 1.23+，模块化单体，单一静态二进制 |
| 数据库 | PostgreSQL 16，唯一有状态组件 |
| 任务队列 | River（跑在 Postgres 上），**不引入 Redis** |
| 前端 | React 18 + TypeScript + Vite + antd 5，构建产物 `go:embed` 进二进制 |
| 前端要求 | 交互逻辑一致：原有每个按钮/能力必须有对应实现，UI 外观可以不同 |
| 会话存档 | 不在本期范围，不做单独存储设计 |
| 部署形态 | 企业私有化单机部署，对标火山引擎/腾讯云轻量云服务器，无信创要求 |
| 数据迁移 | 新服务器全量迁移 → 对账 → 整体切换；客户+标签+时间线必迁，历史群发/问卷提交默认全量迁（可砍） |
| 功能范围 | 只重做已有功能，零新功能。行为以线上 aicrm_next 为准 |
| 容量基准 | 企微客户 10–20 万，销售 5–10 人，运营 2–3 人，单机不做水平扩展设计 |

---

## 1. 部署拓扑与容量规格

### 1.1 单机拓扑

```
┌────────────────── 云服务器（规格按 §1.2 分档选型） ──────────────────┐
│                                                                    │
│  nginx (TLS 终结, 静态兜底)                                         │
│    └── aicrm (Go 单二进制, systemd)                                │
│          ├── --role=api    : HTTP API + embedded 前端 + 企微回调接收 │
│          └── --role=worker : River 任务(外发/人群刷新/同步/统计)      │
│              ※ S 档可合并为单进程 --role=all                        │
│                                                                    │
│  PostgreSQL 16 (同机, systemd)                                     │
│    ├── 业务库 aicrm                                                │
│    └── River 队列表 (同库)                                          │
│                                                                    │
│  备份: 每日 pg_dump → 云对象存储 (TOS/COS), 保留 30 天               │
└────────────────────────────────────────────────────────────────────┘
```

- 交付标准：Docker Compose（app + postgres 两个容器）为默认交付物；同时提供裸机 systemd 方案。
- 20 万客户全库预估 < 10GB（不含会话存档）。磁盘一律 ≥ 60GB SSD（数据 + 备份 + 日志余量），磁盘比 CPU/内存便宜得多，不省。
- **绑定资源是内存，不是 CPU。** 后台 15 人的请求 CPU 开销可忽略；内存不足会导致 Postgres 缓存命中率暴跌、并在极端时被 OOM killer 杀掉（比慢十倍严重）。选型看内存。

### 1.2 服务器规格分档清单（FDE 交付选型表）

**2C4G 为交付最低线**，低于此不承诺 SLA。分档如下：

| 档位 | 规格 | 适用客户 | 日常表现 | 群发 20 万耗时 | 备注 |
|---|---|---|---|---|---|
| **S** | 2C4G / 60GB | < 5 万客户，日常群发 < 2 万 | 流畅 | — | 最低交付线。api+worker 可合并单进程 |
| **M（默认）** | 4C8G / 100GB | 5–20 万客户，常规群发 | 流畅，峰值无感 | 按企微限速匀速，约 1–3h | **标准交付档，报价默认按此** |
| **L** | 8C16G / 200GB | 20 万上限 + 高频大群发 + 多扩展组件 | 峰值仍余量充足 | 同上（瓶颈在企微限速非机器） | 组件多/并发外发多时选 |
| **迁移临时档** | 在原档基础上临时升配一档 | 数据迁移当天 | — | — | 迁完降回。**禁止在 S 档上跑全量迁移** |

**降档警告（2C2G）**：客户坚持 2C2G 时可跑但不承诺 SLA，必须书面告知：群发耗时可能翻数倍、须开 4GB swap、须全部重任务排凌晨。**且迁移必须临时升配**。

### 1.3 各档调优参数（部署脚本按档位自动套用）

| 参数 | S (2C4G) | M (4C8G) | L (8C16G) |
|---|---|---|---|
| `shared_buffers` | 1GB | 2GB | 4GB |
| `effective_cache_size` | 2GB | 5GB | 10GB |
| `work_mem` | 8MB | 16MB | 32MB |
| `maintenance_work_mem` | 128MB | 256MB | 512MB |
| `max_connections` | 40 | 80 | 150 |
| Go `GOMEMLIMIT` | 768MB | 1.5GB | 3GB |
| pgx 池 (api / worker) | 10 / 5 | 20 / 10 | 30 / 20 |
| River worker 并发 | 3 | 8 | 16 |
| 外发批量插入分块 | 1000 | 5000 | 10000 |
| swap | 4GB（必开） | 2GB（建议） | 可不开 |

- 部署脚本接受 `--tier=s|m|l`，自动生成对应的 `postgresql.conf` 与应用环境变量，**杜绝人工调参出错**（这本身也是旧系统"改参数踩坑"的对策之一）。
- S 档必须把人群刷新、统计预聚合、大批量群发排到低峰时段（配置项 `worker.heavy_task_window`，默认 01:00–06:00）。

### 1.4 容量红线（防过度设计）

- 后台并发用户 ≤ 15，API QPS 峰值按 50 设计即可。
- 企微回调峰值按 100/s 设计（加好友活动高峰），Go 单进程直接扛。
- 单次群发上限 20 万条 outbound 任务，worker 按企微限速匀速消费——**群发总耗时的瓶颈是企微 API 限速，不是服务器规格**，升配不会让群发变快，只让群发不影响其他人用。
- **禁止**引入：消息中间件、微服务拆分、读写分离数据库、分库分表。以上任何一项出现在 PR 里即打回。

---

## 2. 数据库设计

### 2.1 设计原则

1. 客户表宽化：高频筛选字段一律做实体列 + 索引，**禁止**在 JSONB 上做筛选条件。
2. 时间线按月分区，append-only。
3. 人群（人群包/Audience）= 定义 + 物化成员表，前台只查成员表。
4. 列表页 keyset 分页，总数走预聚合，禁止 `COUNT(*)` 和深 `OFFSET`。
5. 所有跨模块状态变更必须在同事务写 `event_log`（Transactional Outbox）。

### 2.2 核心 DDL

```sql
-- ============ 客户域 (contact 模块独占写权限) ============

CREATE TABLE staff (                        -- 企微成员(销售/运营)
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  wecom_userid  TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL,
  department    TEXT,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE channels (                     -- 来源渠道(活码/渠道码)
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name          TEXT NOT NULL,
  code          TEXT UNIQUE,                -- 渠道码标识
  config        JSONB NOT NULL DEFAULT '{}',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stages (                       -- 转化阶段定义
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name          TEXT NOT NULL,
  sort_order    INT NOT NULL,
  config        JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE customers (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  external_userid  TEXT NOT NULL UNIQUE,     -- 企微外部联系人ID
  unionid          TEXT,
  name             TEXT NOT NULL DEFAULT '',
  avatar_url       TEXT,
  gender           SMALLINT,
  -- 高频筛选字段: 全部实体列
  stage_id         BIGINT REFERENCES stages(id),
  owner_staff_id   BIGINT REFERENCES staff(id),
  channel_id       BIGINT REFERENCES channels(id),
  added_at         TIMESTAMPTZ,              -- 加好友时间
  last_interact_at TIMESTAMPTZ,              -- 最近互动时间
  is_deleted       BOOLEAN NOT NULL DEFAULT FALSE,  -- 流失/删除
  extra            JSONB NOT NULL DEFAULT '{}',     -- 仅存展示性字段,禁止筛选
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_customers_owner_stage ON customers (owner_staff_id, stage_id) WHERE NOT is_deleted;
CREATE INDEX idx_customers_stage_interact ON customers (stage_id, last_interact_at DESC) WHERE NOT is_deleted;
CREATE INDEX idx_customers_channel ON customers (channel_id) WHERE NOT is_deleted;
CREATE INDEX idx_customers_unionid ON customers (unionid) WHERE unionid IS NOT NULL;

CREATE TABLE tag_groups (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL, sort_order INT NOT NULL DEFAULT 0
);
CREATE TABLE tags (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  group_id      BIGINT REFERENCES tag_groups(id),
  name          TEXT NOT NULL,
  wecom_tag_id  TEXT UNIQUE                 -- 同步到企微的标签ID,可空(纯本地标签)
);
CREATE TABLE customer_tags (
  customer_id BIGINT NOT NULL REFERENCES customers(id),
  tag_id      BIGINT NOT NULL REFERENCES tags(id),
  tagged_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  tagged_by   TEXT NOT NULL DEFAULT 'system',   -- system/staff:{id}/automation:{id}
  PRIMARY KEY (customer_id, tag_id)
);
CREATE INDEX idx_customer_tags_tag ON customer_tags (tag_id, customer_id);

-- 时间线: 按月分区 append-only
CREATE TABLE customer_events (
  id          BIGINT GENERATED ALWAYS AS IDENTITY,
  customer_id BIGINT NOT NULL,
  event_type  TEXT NOT NULL,      -- added/deleted/tag_applied/tag_removed/stage_changed/
                                  -- message_sent/survey_submitted/note/... 与旧系统枚举对齐
  payload     JSONB NOT NULL DEFAULT '{}',
  actor       TEXT NOT NULL DEFAULT 'system',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (occurred_at, id)
) PARTITION BY RANGE (occurred_at);
CREATE INDEX idx_ce_customer_time ON customer_events (customer_id, occurred_at DESC);
CREATE INDEX idx_ce_brin ON customer_events USING BRIN (occurred_at);
-- 分区由启动任务自动预建未来3个月, 迁移脚本按历史数据范围建

-- ============ 人群域 (segment 模块, 对应旧系统"AI Audience/人群包") ============

CREATE TABLE segments (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name           TEXT NOT NULL,
  definition     JSONB NOT NULL,        -- 声明式筛选 DSL, 见 §3.3
  refresh_mode   TEXT NOT NULL DEFAULT 'manual',  -- manual/scheduled
  refresh_cron   TEXT,
  member_count   INT NOT NULL DEFAULT 0,          -- 冗余, 刷新时更新
  refreshed_at   TIMESTAMPTZ,
  refresh_status TEXT NOT NULL DEFAULT 'idle',    -- idle/running/failed
  created_by     BIGINT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE segment_members (
  segment_id  BIGINT NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  customer_id BIGINT NOT NULL,
  computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (segment_id, customer_id)
);

-- ============ 自动化域 (automation 模块) ============

CREATE TABLE automations (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name           TEXT NOT NULL,
  trigger_type   TEXT NOT NULL,     -- event:{event_type} / schedule / segment_enter
  trigger_config JSONB NOT NULL DEFAULT '{}',
  conditions     JSONB NOT NULL DEFAULT '[]',   -- 与 segment DSL 同一套条件语法
  actions        JSONB NOT NULL DEFAULT '[]',   -- [{type: send_msg|apply_tag|move_stage|ai_generate, ...}]
  enabled        BOOLEAN NOT NULL DEFAULT FALSE,
  version        INT NOT NULL DEFAULT 1,        -- 每次修改+1, enrollment 记录版本
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE automation_enrollments (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  automation_id   BIGINT NOT NULL REFERENCES automations(id),
  automation_ver  INT NOT NULL,
  customer_id     BIGINT NOT NULL,
  source_event_id BIGINT,
  idempotency_key TEXT NOT NULL UNIQUE,   -- {automation_id}:{customer_id}:{event_id 或 日期}
  status          TEXT NOT NULL DEFAULT 'pending', -- pending/done/skipped/failed
  result          JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_enroll_auto ON automation_enrollments (automation_id, created_at DESC);

-- ============ 外发域 (outbound 模块独占企微写API) ============

CREATE TABLE outbound_batches (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_type  TEXT NOT NULL,      -- manual/segment/automation
  source_id    BIGINT,
  msg_template JSONB NOT NULL,     -- 消息内容模板(文本/图片/链接/小程序)
  total        INT NOT NULL DEFAULT 0,
  sent         INT NOT NULL DEFAULT 0,
  failed       INT NOT NULL DEFAULT 0,
  status       TEXT NOT NULL DEFAULT 'pending', -- pending/running/done/cancelled
  created_by   BIGINT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE outbound_tasks (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  batch_id        BIGINT REFERENCES outbound_batches(id),
  task_type       TEXT NOT NULL,   -- external_msg/group_msg/apply_wecom_tag/moment...
                                   -- 枚举与旧系统 WeCom External Effect 类型对齐
  customer_id     BIGINT,
  sender_staff_id BIGINT,
  payload         JSONB NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  status          TEXT NOT NULL DEFAULT 'pending', -- pending/sending/sent/failed/cancelled
  attempts        INT NOT NULL DEFAULT 0,
  next_retry_at   TIMESTAMPTZ,
  wecom_msgid     TEXT,
  last_error      TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at         TIMESTAMPTZ
);
CREATE INDEX idx_ot_pending ON outbound_tasks (status, next_retry_at) WHERE status IN ('pending','failed');
CREATE INDEX idx_ot_batch ON outbound_tasks (batch_id, status);

-- ============ 事件总线 (events 模块, Transactional Outbox) ============

CREATE TABLE event_log (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type   TEXT NOT NULL,
  customer_id  BIGINT,
  payload      JSONB NOT NULL DEFAULT '{}',
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatched   BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_el_undispatched ON event_log (id) WHERE NOT dispatched;

-- ============ 问卷域 (survey 模块) ============

CREATE TABLE surveys (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  title      TEXT NOT NULL,
  definition JSONB NOT NULL,        -- 题目结构, 与旧系统问卷 schema 对齐
  status     TEXT NOT NULL DEFAULT 'draft',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE survey_submissions (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  survey_id    BIGINT NOT NULL REFERENCES surveys(id),
  customer_id  BIGINT,              -- 通过 openid/unionid 归因, 可空
  openid       TEXT,
  answers      JSONB NOT NULL,
  submitted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ss_survey ON survey_submissions (survey_id, submitted_at DESC);
-- (原 oauth_bindings 已废除, 公众号 openid 归因统一走身份域 identities, 见下)

-- ============ 身份域 (identity 模块独占写权限, 设计详见 §7) ============

CREATE TABLE identities (           -- 身份图谱: 一个客户挂 N 条外部标识
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id BIGINT REFERENCES customers(id),  -- 可空 = 游离身份(未归因, 等待匹配)
  id_type     TEXT NOT NULL,        -- wecom_external_userid / unionid / mp_openid /
                                    -- oa_openid / alipay_user_id / phone / ext:{自定义}
  scope       TEXT NOT NULL DEFAULT '',  -- 区分同类型不同主体: openid 填 appid,
                                         -- 支付宝填应用ID, phone/unionid 留空
  id_value    TEXT NOT NULL,
  confidence  TEXT NOT NULL DEFAULT 'declared', -- verified(验证过,如短信验证/官方接口返回)
                                                -- declared(用户自填/表单)
  source      TEXT NOT NULL,        -- 写入方: wecom/survey/ext:{extension_key}
  bound_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (id_type, scope, id_value)
);
CREATE INDEX idx_identities_customer ON identities (customer_id) WHERE customer_id IS NOT NULL;

CREATE TABLE customer_merges (      -- 合并审计: 只追加, 不可删
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  primary_id   BIGINT NOT NULL,     -- 保留的主记录
  merged_id    BIGINT NOT NULL,     -- 被并入的记录(customers 行保留, is_deleted=true 并标记)
  match_key    TEXT NOT NULL,       -- 触发合并的标识: unionid:{v} / phone:{v} / manual
  mode         TEXT NOT NULL,       -- auto/manual
  operated_by  TEXT NOT NULL,
  detail       JSONB NOT NULL DEFAULT '{}',
  merged_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pending_events (       -- 归因失败的外部事件暂存, 身份补齐后回放
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  refs        JSONB NOT NULL,       -- 事件携带的外部标识列表 [{id_type,scope,id_value}]
  event_type  TEXT NOT NULL,
  payload     JSONB NOT NULL,
  source      TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  status      TEXT NOT NULL DEFAULT 'pending',  -- pending/attributed/expired
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pe_pending ON pending_events (status) WHERE status = 'pending';

-- ============ AI 域 (ai 模块: 只做生成, 不做决策) ============

CREATE TABLE ai_prompts (
  key         TEXT NOT NULL,
  version     INT NOT NULL,
  template    TEXT NOT NULL,
  model_config JSONB NOT NULL DEFAULT '{}',  -- provider/model/temperature
  is_active   BOOLEAN NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key, version)
);
CREATE TABLE ai_generations (       -- 全量审计
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  prompt_key  TEXT NOT NULL,
  prompt_ver  INT NOT NULL,
  customer_id BIGINT,
  input       JSONB NOT NULL,
  output      TEXT,
  usage       JSONB,               -- tokens/latency/cost
  status      TEXT NOT NULL,       -- ok/failed
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 配置域 (config 模块: 单一事实源) ============

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,     -- 命名空间化: wecom.corp_id / ai.provider / outbound.rate_limit
  value      JSONB NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE settings_audit (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key TEXT NOT NULL, old_value JSONB, new_value JSONB NOT NULL,
  updated_by TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 后台账号 / 统计 ============

CREATE TABLE admin_users (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
  role TEXT NOT NULL,              -- admin/ops/sales
  staff_id BIGINT REFERENCES staff(id),   -- sales 角色绑定企微成员, 数据范围=自己客户
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stats_daily (          -- 报表/看板只读这张表
  stat_date  DATE NOT NULL,
  metric_key TEXT NOT NULL,         -- customer_added/customer_lost/msg_sent/stage_count...
  dims       JSONB NOT NULL DEFAULT '{}',  -- {staff_id, channel_id, stage_id...}
  value      NUMERIC NOT NULL,
  PRIMARY KEY (stat_date, metric_key, dims)
);

CREATE TABLE wecom_sync_state (     -- 企微同步游标
  resource TEXT PRIMARY KEY,        -- contacts/staff/tags/groups
  cursor   TEXT,
  synced_at TIMESTAMPTZ
);
```

### 2.3 查询规范（写进 CI 检查）

- 所有列表接口签名统一为 `(filter, cursor, limit)`，返回 `{items, next_cursor}`；游标 = `(排序列值, id)` 编码。
- 客户列表页总数显示：≤1 万精确（`count` 走索引），>1 万显示"约 N"（`pg_class.reltuples` 估算或 stats_daily）。
- 每个新增查询必须附 `EXPLAIN` 结果进 PR，出现 Seq Scan 于 customers/customer_events 即打回。
- sqlc 生成全部查询代码，**禁止**手拼 SQL 字符串、禁止 ORM。

---

## 3. 模块架构与接口契约

### 3.1 目录结构

```
/cmd/aicrm/main.go              # 唯一入口, --role=api|worker|all
/api/openapi.yaml               # ★ 唯一 API 契约, spec-first
/internal
  /contact                      # 客户/标签/阶段/时间线 —— customers 表唯一写入方
  /identity                     # 身份图谱 resolve/bind/ingest/merge —— identities 表唯一写入方 (§7)
  /segment                      # 人群包定义/物化/刷新 worker
  /automation                   # 触发-条件-动作, 只消费事件、只产出 outbound 任务
  /outbound                     # Outbox + 企微限速发送 worker —— 企微写API唯一出口
  /wecom                        # 回调接收/验签、通讯录与客户同步 —— 企微读API唯一出口
  /ai                           # 内容生成(火山引擎为主, provider 接口可换), 无状态
  /survey                       # 问卷 + 公众号 OAuth
  /gateway                      # 对外集成边界: MCP/OpenClaw(承接旧 integration_gateway)
                                #   + Extension API(定制组件接入面, §7.4)
  /config                       # 强类型配置注册表
  /events                       # event_log 写入 + dispatcher
  /auth                         # 后台账号/RBAC/数据范围
  /stats                        # 每日预聚合 worker
/internal/platform              # db(pgx)/river/httpserver/logging 等基础设施
/web                            # React 前端, 构建产物 embed
/migrations                     # goose SQL 迁移
/tools/migrate-from-v1          # 旧库迁移工具(独立可执行)
/tools/contract-replay          # 契约回放测试工具
```

### 3.2 铁律（go-arch-lint 强制，CI 不过不合并）

1. 模块间禁止 import 对方内部包，只允许 import 对方 `port.go`（显式接口）或发事件。
2. `customers` / `customer_tags` / `customer_events` 只允许 contact 模块写。
3. 企微写 API 只允许 outbound 模块调用；企微读 API 只允许 wecom 模块调用。
4. 任何状态变更 handler 必须在同事务写 `event_log`。
5. 配置只能通过 config 模块的强类型 struct 读取，禁止散落的 `os.Getenv` / 直查 settings 表。
6. `identities` / `customer_merges` / `pending_events` 只允许 identity 模块写；任何模块（含 wecom、survey）拿到外部标识后只能调 identity 的 `Resolve/Bind/Ingest`，禁止自建"openid → 客户"之类的映射表。
7. 定制扩展组件（支付/商品/外部小程序等）永不进入本仓库，只能通过 gateway 的 Extension API 接入（§7.4）。
8. 禁止用 `time.Ticker` / `time.AfterFunc` / 第三方 cron 库做业务周期任务，一律走 River periodic job（§8.2）；go-arch-lint 扫这些符号，出现即 CI 红。

### 3.3 关键契约定义

**筛选条件 DSL**（segment 与 automation 条件共用同一套，避免两套逻辑漂移）：

```json
{
  "and": [
    {"field": "stage_id", "op": "in", "value": [2, 3]},
    {"field": "tag_id",   "op": "has_any", "value": [15, 22]},
    {"field": "last_interact_at", "op": "before", "value": "-7d"},
    {"field": "channel_id", "op": "eq", "value": 4}
  ]
}
```

- 可筛字段白名单 = customers 实体列 + tag。DSL 编译器把它翻译成走索引的 SQL，新增字段必须同时加列、加索引、进白名单。
- 编译器是唯一一处"筛选 → SQL"转换代码，segment 刷新和 automation 条件判断都调它。

**领域事件枚举**（v1 冻结，与旧系统时间线事件对齐后定稿）：

```go
// internal/events/types.go
const (
    EvCustomerAdded     = "customer.added"
    EvCustomerDeleted   = "customer.deleted"
    EvTagApplied        = "customer.tag_applied"
    EvTagRemoved        = "customer.tag_removed"
    EvStageChanged      = "customer.stage_changed"
    EvSurveySubmitted   = "survey.submitted"
    EvOutboundSent      = "outbound.sent"
    EvOutboundFailed    = "outbound.failed"
)
```

**模块接口示例**（每个模块一个 `port.go`，这是模块间唯一合法依赖面）：

```go
// internal/contact/port.go
type Service interface {
    Get(ctx context.Context, id int64) (Customer, error)
    List(ctx context.Context, q ListQuery) (Page[Customer], error)
    ApplyTags(ctx context.Context, customerID int64, tagIDs []int64, actor string) error
    RemoveTags(ctx context.Context, customerID int64, tagIDs []int64, actor string) error
    MoveStage(ctx context.Context, customerID, toStageID int64, actor string) error
    Timeline(ctx context.Context, customerID int64, cursor string, limit int) (Page[Event], error)
}

// internal/outbound/port.go
type Enqueuer interface {
    EnqueueBatch(ctx context.Context, b BatchSpec) (batchID int64, err error)  // 群发
    EnqueueOne(ctx context.Context, t TaskSpec) error                          // 单条(自动化用)
}

// internal/ai/port.go  —— 无状态, 决策逻辑不进这里
type Generator interface {
    Generate(ctx context.Context, promptKey string, input map[string]any) (Result, error)
}
```

**强类型配置**（治"改 A 坏 B"的最后一环）：

```go
// internal/config/schema.go —— 全部配置项集中定义, 启动时全量校验, 失败拒绝启动
type Root struct {
    WeCom    WeComCfg    `settings:"wecom"`
    AI       AICfg       `settings:"ai"`
    Outbound OutboundCfg `settings:"outbound"`
    Survey   SurveyCfg   `settings:"survey"`
}
type WeComCfg struct {
    CorpID         string `json:"corp_id" validate:"required"`
    AgentID        int    `json:"agent_id" validate:"required"`
    Secret         string `json:"secret" validate:"required"`
    CallbackToken  string `json:"callback_token" validate:"required"`
    CallbackAESKey string `json:"callback_aes_key" validate:"required,len=43"`
}
type OutboundCfg struct {
    RatePerSecond int `json:"rate_per_second" validate:"min=1,max=50"` // 企微限速
    MaxAttempts   int `json:"max_attempts" validate:"min=1,max=10"`
}
```

- 后台"系统设置"页保存时按同一 struct 校验；每次变更写 settings_audit。
- 配置热更新：保存后发进程内信号重载，无需重启。

### 3.4 核心链路时序

**群发链路**：运营选人群包 → `outbound.EnqueueBatch`（展开 segment_members 为 N 条 task，事务内完成）→ River worker 按 `outbound.rate_per_second` 令牌桶消费 → 调企微 API → 更新 task 状态 + 写 `outbound.sent` 事件 → contact 模块消费事件写时间线。

**自动化链路**：业务写库 + 同事务写 event_log → dispatcher（River 周期任务）取未分发事件 → 匹配启用的 automations（trigger_type 索引匹配）→ 条件 DSL 判断 → 生成 enrollment（幂等键去重）→ 执行 action（apply_tag 调 contact 接口 / send_msg 调 outbound / ai_generate 先调 ai 再调 outbound）。

**AI 链路**：自动化或人工触发 → ai.Generate（读 ai_prompts 活跃版本 → 调火山引擎，provider 接口抽象可换供应商）→ 写 ai_generations 审计 → 产物作为消息内容交给 outbound。**AI 永远不直接决定"发给谁/何时发"，只生成"发什么"**。

---

## 4. 前端重构方案

### 4.1 技术栈与工程

- React 18 + TypeScript + Vite + antd 5 + TanStack Query（服务端状态）+ zustand（少量本地状态）。
- API client 从 `openapi.yaml` 用 orval 自动生成，**前端禁止手写 fetch**——前后端唯一契约就是这份 spec，字段改名会在前端编译期报错。
- 构建产物 `go:embed`，无独立前端部署物。

### 4.2 "交互逻辑一致"的落地方法

1. 从旧系统前端代码 + 线上页面，用 AI 提取**功能矩阵表**：`页面 → 区块 → 按钮/操作 → 调用的API → 预期结果`，每行一条。人工核对后冻结，作为唯一验收清单（预估 150–300 行）。
2. 新前端每个页面的 PR 必须引用功能矩阵行号，标注"已覆盖"；矩阵全部打勾 = 前端完成的定义。
3. UI 外观自由发挥（antd 默认风格即可），但操作路径不允许比旧版多步。

### 4.3 页面清单（按旧系统六大块，最终以功能矩阵为准）

后台控制台（登录/账号/角色/系统设置）、客户中心（列表/详情/时间线/标签/阶段看板）、人群包管理（本期重点补齐的旧缺口：列表/新建筛选/刷新/成员预览）、自动化（规则列表/编辑器/enrollment 记录）、群发（创建/进度/失败重发）、问卷（列表/编辑/提交数据/OAuth 配置）、数据看板（stats_daily 驱动）。

---

## 5. 行为基准提取与契约测试

以线上 aicrm_next 实际行为为准，三步固化：

1. **API 清单**：旧系统 `python3 app.py routes` 导出全部路由 → 逐条映射到新 `openapi.yaml`，产出对照表（旧路由 → 新路由 → 差异说明，差异只允许是路径风格，不允许是语义）。
2. **Golden case 录制**：在旧系统 nginx 加镜像日志（或 FastAPI middleware）录制 1–2 周真实请求/响应，脱敏后按接口归档为回放用例；低频接口人工补造用例。目标：每个接口 ≥ 3 个 case（正常/边界/错误）。
3. **契约回放**：`tools/contract-replay` 把 golden case 打到新系统，逐字段 diff（忽略时间戳/ID 等白名单字段）。回放通过率 100% 是切换的硬性前置条件。

---

## 6. 数据迁移与切换（新服务器原则）

### 6.1 迁移范围

| 数据 | 处理 |
|---|---|
| 客户 + 标签 + 阶段 + 归属 | 必迁，schema 映射转换 |
| 时间线 | 必迁，按历史时间范围自动建分区后灌入 |
| 历史群发记录 | 默认全量迁（映射进 outbound_batches/tasks，状态置终态） |
| 旧问卷 + 提交 | 默认全量迁 |
| 旧 OAuth 绑定 / 各类 ID 映射 | 必迁，统一转换为 identities 记录（openid/unionid/手机号各成一条边，confidence 按来源标注） |
| 系统配置 | 不自动迁，人工在新后台按新 schema 重新录入并校验 |
| 会话存档 | 不涉及 |

### 6.2 迁移工具与流程

`tools/migrate-from-v1`：直连旧 Postgres 只读抽取 → 转换 → 写新库，支持 `--full` 与 `--incremental --since=<ts>`（基于旧表 updated_at），可反复执行（upsert 幂等）。

```
T-7d   新服务器部署 v2, 全量迁移演练 ×2, 对账脚本跑通
T-1d   全量迁移正式执行, 契约回放全绿
T日    低峰期(凌晨): 旧系统停写(挂维护页, 预计 ≤30min)
       → 增量迁移(分钟级) → 对账(行数 + 关键聚合 + 100条抽样逐字段)
       → 企微后台切换: 回调URL/可信域名/JS域名指向新服务器 → 验签通过
       → nginx/DNS 切流量 → 冒烟checklist(加好友回调/打标/群发1条/问卷提交)
T+30d  旧服务器只读保留, 到期销毁
```

回滚预案：切换后 24h 内发现阻断问题 → 企微回调与 DNS 切回旧服务器（旧系统未动，随时可回），期间新库产生的增量数据人工评估处理。

---

## 7. 身份图谱与扩展组件接入（FDE 多组件基座）

> 设计参照：Segment 的 Identify/Track/Alias 规范（三动词接入模型）、阿里 OneID / 神策 ID-Mapping（内部主键 + 外部标识分离）、Salesforce Data Cloud Identity Resolution（match rules + 合并置信度分级）。

### 7.1 核心原则

1. **`customers.id` 就是 OneID**，渠道中立，不等于任何外部 ID。企微 external_userid 与 alipay_user_id、手机号一样，只是 `identities` 表里的一条边——企微是权重最高的渠道，但不是身份体系的主键。
2. **匹配键分级**：`unionid`（微信生态内打通，需各客户企微后台绑定微信开放平台后才能从外部联系人详情获取——这是 FDE 落地 checklist 必查项）> `phone + verified`（跨生态打通）> 各 openid（仅在各自 scope 内有效）。`declared` 级手机号（用户自填未验证）只做弱关联提示，不触发自动归并。
3. **核心系统内任何模块不得自建 ID 映射**（铁律 6）。wecom 模块拿到 external_userid/unionid、survey 模块拿到 openid/手机号，一律调 identity 模块登记。

### 7.2 identity 模块接口

```go
// internal/identity/port.go
type IDRef struct { Type, Scope, Value string }

type Service interface {
    // 外部标识 → 客户。查不到返回 found=false, 不隐式建档
    Resolve(ctx context.Context, ref IDRef) (customerID int64, found bool, err error)
    // 绑定标识到客户。若该标识已绑定在另一客户上 → 触发 §7.3 合并流程
    Bind(ctx context.Context, customerID int64, ref IDRef, confidence, source string) error
    // 组件上报事件 + 其已知的全部标识: 能归因→写时间线并顺手 Bind 新标识;
    // 不能归因→存 pending_events, 后续任何 Bind 成功后由 worker 自动回放
    Ingest(ctx context.Context, refs []IDRef, eventType string, payload map[string]any,
           source string, occurredAt time.Time) error
}
```

`Ingest` 是外部组件的主入口：支付组件收到支付宝回调，只需要把"支付成功"事件连同它手里的 `alipay_user_id` + 订单手机号一起上报，归因是核心的事，组件不需要理解企微。

### 7.3 合并规则（默认策略，附录 B 记录）

| 触发情形 | 处理 |
|---|---|
| Bind 时发现 `unionid` 已在另一客户名下 | **自动合并**：保留信息更全者为主记录（企微在档 > 仅游离身份），从记录 `is_deleted=true` 并写 `customer_merges`；标签取并集、时间线归并（事件带原始 customer_id 存 payload 备查）、segment_members 下次刷新自然修正 |
| Bind 时发现 `phone(verified)` 冲突 | **进人工待办**（后台"待合并"列表，运营确认后手动合并）——手机号存在换主人的现实可能 |
| `phone(declared)` 或 openid 冲突 | 不合并，仅在客户详情页展示"疑似同人"提示 |
| 任何自动合并 | 发 `customer.merged` 事件（供扩展组件 webhook 订阅更新其本地引用） |

合并只有"归并"没有物理删除；`customer_merges` 永久可追溯。拆分（误合并回退）为人工工单操作，v1 不做自动化。

### 7.4 Extension API（定制组件唯一接入面，gateway 模块承载）

**组件形态定稿**：每个 FDE 定制组件（支付、商品、外部小程序后端等）= 独立小仓库 + 独立小服务，与核心同机部署（Compose 加一个容器），语言不限。核心仓库保持产品化统一版本，定制件按客户各自演进——两者只通过以下 HTTP 面交互：

```
POST /ext/v1/identity/resolve      # IDRef → customer_id
POST /ext/v1/identity/bind         # 绑定标识(需声明 confidence/source)
POST /ext/v1/events/ingest         # 上报事件+标识, 核心归因落时间线
GET  /ext/v1/customers/{id}        # 客户档案只读(字段级脱敏按 key 权限配置)
POST /ext/v1/outbound/enqueue      # 组件触发发消息(经核心限速队列, 不许自己调企微)
GET  /ext/v1/webhooks              # 订阅 customer.merged / tag_applied 等事件回推
```

鉴权：每组件一个 API key（settings 管理），key 绑定权限范围与脱敏级别；全部调用写审计。事件经 Ingest 落时间线后，automation 模块即可对"支付成功"这类外部事件配触发规则——**扩展组件天然获得自动化能力，无需自己实现**。

### 7.5 本期范围界定（守住"只重做已有功能"）

本期 v2 落地的最小集：`identities`/`customer_merges`/`pending_events` 三张表、identity 模块（Resolve/Bind/Ingest + unionid 自动合并 + pending 回放 worker）、旧 oauth_bindings 迁移转换、Extension API 六个端点骨架 + API key 鉴权、后台"待合并"列表页。**支付、商品等任何具体扩展组件都不在本期**——它们是后续各 FDE 项目的独立交付物，本期只保证插座立好。

---

## 8. 运维与可观测性（针对旧系统四类顽疾的结构性根治）

> 本章直接对应旧系统暴露的四个运维问题。核心判断：四个问题同一个根因——**进程内什么都自己扛（Web 请求 + 散落 timer + 外发 + 同步全挤在一个进程一个连接池）**。方案已有的"River 队列 + api/worker 角色可拆"是主解，本章把它配置到位并补齐可观测手段。可观测性定档"够用"：一个内置运维页 + 结构化接口日志，**不引入 Prometheus/Grafana**（守 §1.4 单机不过度红线）。

### 8.1 问题①：多人互相拖慢加载

根因：用户请求与后台重活抢同一进程资源。三层解法：

1. **角色隔离**：生产部署跑两个进程——`--role=api`（只服务后台请求）与 `--role=worker`（只跑队列任务），同机不同进程，各自独立连接池。后台跑批/同步/群发再重也不占用户请求资源。单机双进程，不违反"不做水平扩展"。
2. **单账号并发预算**：API 中间件对每个后台账号限制在途请求数（默认 4，settings 可调），一个人狂点/开多标签页不会耗尽连接池拖垮他人。超限返回 429 + 前端排队提示，而非无声变慢。
3. **侧边栏/菜单/权限缓存**：这类每次进后台都要拉、且很少变的数据，Go 进程内 LRU 缓存（TTL 60s，变更时主动失效）。侧边栏慢的典型病因就是"每次渲染都跑一堆权限判定 + 未读计数聚合查询"，缓存后首屏查询数从几十降到个位数。

### 8.2 问题②：timer 过多、轮询卡队列查不到数据

根因：散落的 APScheduler/线程 timer 自转，互相踩、无锁、无可观测。**彻底移除所有自建 timer**，全部周期任务收敛为 River periodic job：

- 全系统周期任务清单（唯一入口 `internal/platform/scheduler.go` 注册）：segment 定时刷新、stats 每日预聚合、wecom 增量同步、outbound 重试扫描、pending_events 回放、event_log dispatcher、customer_events 分区预建。
- River 天然保证：**同一任务全局唯一锁**（不会并发跑两份）、失败自动重试带退避、每个任务在任务表里有明确状态（pending/running/completed/failed/scheduled）。
- **铁律补充（写入 §3.2）**：禁止任何 `time.Ticker` / `time.AfterFunc` / 第三方 cron 库做业务周期任务，一律走 River periodic job。go-arch-lint 加规则扫这些符号，出现即 CI 红。
- 这直接根治"轮询卡队列查不到具体数据"——River 任务表 + §8.4 运维页让每个任务卡在哪、卡多久、什么错误一目了然。

### 8.3 问题③：API 访问不了、要清晰的 API 管理

根因：接口分散、无统一治理、坏了没人知道。三样补齐（旧问题接口不专门挖清单，靠 §5 录制回放自然覆盖）：

1. **spec-first 单一真相**：`openapi.yaml` 是唯一接口定义，server stub 与前端 client 都由它生成，接口签名不一致在编译期暴露（已在 §3.1）。
2. **统一 API 网关中间件**（所有 `/api/*` 业务接口强制经过，顺序固定）：请求ID注入 → 鉴权 → 单账号并发预算(§8.1) → 超时控制(默认 10s，可按接口覆盖) → panic 恢复 → 统一错误码结构 → 结构化访问日志(`request_id/account/method/path/status/latency_ms/err`)。任一接口异常，凭 request_id 在日志秒级定位，不再"各种原因访问不了"却查不到原因。
3. **内置接口健康自检**（§8.4 运维页内）：对核心业务接口做浅层探活（依赖 DB/队列/企微 token 有效性），后台可见每个核心能力的可用性，坏了主动发现而非等用户报障。

### 8.4 内置运维页（够用档，后台 admin 角色可见）

单页三块，全部读现成数据、不额外建监控设施：

- **队列面板**：读 River 任务表——各任务类型的 pending/running/failed 计数、最近失败任务列表（含错误摘要）、periodic job 上次执行时间与状态、单个失败任务"重试/取消"按钮。这块直接替代旧系统里"看不见的 timer"。
- **接口日志面板**：读结构化访问日志——按 status/path/account 过滤，高延迟(>1s)与错误(5xx/429)接口置顶，点开看单条 request_id 全链路。
- **健康自检面板**：DB 连接、River 队列、企微 token、AI provider、各扩展组件 Extension API key 的实时可用性红绿灯。

### 8.5 问题④：改新功能把老功能改回

根因：无回归防线。结构性防御已有（铁律 + go-arch-lint + 契约回放），本节把回放从"切换前跑一次"升级为**CI 常态门**：

- **契约回放进 CI**（升级 §5 与 T0.3）：每个 PR 合并前，对该 PR 触及模块的 golden case 全量回放，任一用例行为变化即红灯，当场拦住"回改"。这是根治问题④的唯一硬手段——结构约束防的是"乱 import"，回放防的是"行为悄悄变了"。
- golden case 随功能矩阵增长持续补充，形成"行为快照库"；任何对既有接口的语义改动必须显式更新对应 golden case（PR 里能看到 diff），杜绝无意识回改。

---

## 9. 里程碑

| 阶段 | 周期 | 交付物 | 出口标准 |
|---|---|---|---|
| M1 行为冻结 | W1–2 | 功能矩阵表、API 对照表、openapi.yaml v1、本文档表结构评审定稿 | 三份清单人工签字冻结 |
| M2 骨架 | W2–3 | 工程脚手架、config/events/auth、API网关中间件、api/worker角色隔离、CI(arch-lint含禁timer + sqlc + EXPLAIN + 契约回放门) | 铁律检查在 CI 生效 |
| M3 核心域 | W3–6 | contact / identity / wecom 同步 / segment / outbound | 20万模拟数据下列表 P95<200ms、群发压测通过 |
| M4 上层域 | W5–8 | automation / survey / ai / gateway(含 Extension API) / stats / 运维页 + 前端全部页面 | 功能矩阵 100% 打勾 |
| M5 验证 | W8–9 | 契约回放全绿、迁移演练 ×2、对账脚本 | 回放通过率 100% |
| M6 切换 | W10 | 首个客户环境迁移切换 | 冒烟 checklist 通过, 24h 无阻断 |

---

## 附录 A：技术选型清单（锁版本后进 go.mod）

| 用途 | 选型 |
|---|---|
| HTTP | chi + oapi-codegen（spec-first 生成 server stub） |
| DB 访问 | pgx v5 + sqlc |
| 队列 | riverqueue/river |
| 迁移 | goose |
| 企微 SDK | ArtisanCloud/PowerWeChat v3 |
| 校验 | go-playground/validator |
| 架构守护 | go-arch-lint |
| 前端 | React 18 / Vite / antd 5 / TanStack Query / orval |

## 附录 B：默认决策记录（可推翻）

1. 历史群发记录、旧问卷提交：默认全量迁移（新服务器模式下成本极低）。
2. 报表精确度：>1 万的计数显示估算值。
3. 系统配置不自动迁移，切换时人工重录（借机做一次配置清洗，旧配置散乱正是本次重构动因）。
4. 前端 UI 直接用 antd 默认设计语言，不做视觉定制。
5. 身份合并策略：unionid 冲突自动合并、verified 手机号冲突进人工待办、declared 级仅提示（§7.3）。
6. FDE 定制组件形态：独立小仓库 + 独立小服务同机部署，只走 Extension API + API key，永不进核心仓库（§7.4）。
7. 服务器规格：2C4G(S) 为交付最低线，4C8G(M) 为标准交付默认档；2C2G 可跑但不承诺 SLA；迁移当天必须临时升配（§1.2）。
8. 性能验收基准统一在 S 档跑——最低线达标则高档位自然达标。

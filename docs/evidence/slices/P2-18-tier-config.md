# P2-18 分档配置与部署边界证据

## 实现与边界

- 基线：`49d870719d2528aa4e6a05cab54bb90085691b82`
- 分支：`slice/p2-18-tier-deploy`
- `aicrm-config --tier=s|m|l` 以固定表生成 `postgresql.conf`
  与不含凭据的 `aicrm.env`；输出目录为 `0750`，文件为
  `0640`，同目录临时文件 `fsync` 后原子替换。
- S/M/L 的 API 池、worker 池、六队列并发、外发分块与
  PostgreSQL 参数逐项匹配设计表；worker 池分别为
  `9/18/30`，队列合计分别为 `7/15/26`。
- swap 语义明确区分为 S `4096 MiB / required`、M
  `2048 MiB / recommended`、L `0 / optional`；只有 S 档
  `--apply` 会 fail-closed 校验已启用 swap。
- Compose 只有同一 app image 与 PostgreSQL `16.14-bookworm`；S 用
  `role=all`，M/L 用隔离的 `role=api` 和 `role=worker`。
- staging 脚本默认只 render；`--apply` 必须显式授权并从
  外部注入 image、DSN 与 PostgreSQL 密码。脚本不执行 migration。

## 本地验证

- `go test -race -count=1 ./internal/platform/deployment ./cmd/aicrm-config`：
  PASS。
- S/M/L 连续生成两次 byte-for-byte 无 diff，权限与逐项参数：
  PASS。
- 永久负例：凭据键、worker 池不足、缺队列、default queue、
  Compose 额外有状态件、未授权 Docker 调用均被拒绝。
- 授权 apply 形状仅以本地 fake Docker 验证
  `compose version/config/up` 调用序列；这不是真实部署证据。
- 全量 Go/Web/repo-contract/gitleaks 与 GitHub CI 由本片 PR 和
  ledger 绑定，不在执行前预填 PASS。

## correction

- `slice_induced=2`：首轮 CLI 负例把 usage 里的固定 `--tier`
  误判为输入泄露，改为只断言实际恶意值不回显；自审发现
  M 档建议 swap 被字段名误表述为 required，改为目标值与策略
  分离，只有 S 强制。
- `infra_induced=1`：repo-contract 新增的 race target 静态断言首版
  匹配字面 `go test`，但 Makefile 按既有工具链约定使用
  `$(GO) test`；改为精确匹配既有形状，race 与测试范围不变。
- `scope_induced=1`：冻结卡要求新增分档生成器，但漏列了其
  架构模块归属；全量 arch-import lint fail-closed 拒绝顶层
  `internal/deployment`。按既有 canonical 将基础设施收口到
  `internal/platform/deployment`，未把 deployment 伪装为业务域，也未放宽
  未知模块门禁。
- `verification_induced=3`：focused 命令首次未用短路连接，无法
  作为单一绿门证据，随后从头严格重跑；展开生成文件时携带
  临时目录删除 trap 被安全策略拒绝，改用不删除的隔离目录完成
  只读复核；一次过宽 ledger 补丁误改历史 P1-C03 计数，在推送前
  由 base diff 发现并恢复原值。

## 外部门

- staging 部署与浏览器登录/stages CRUD：`PENDING_EXTERNAL_GATE`。
- 生产数据库、live migration、真实企微：`NOT EXECUTED`。

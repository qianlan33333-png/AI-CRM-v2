# P2-17 stages UI 证据

## 实现与边界

- 基线：`9ebd074c68381b2a2a5f8c15ecd1c2cb38a9f45d`
- 分支：`slice/p2-17-stages-ui`
- `/stages` 已接入 P2-16 生成的 Orval client；列表保留服务端顺序，
  响应结构非法时 fail-closed。
- admin/ops 可新增与改名，sales 只读；写请求只从现有
  `aicrm_csrf` cookie 取精确 43 字符 token，缺失时本地拒绝。
- 不使用 optimistic mutation；401 使父 shell 失效 session/cache，其余
  受控失败使用稳定中文提示，不回显服务端错误或 token。
- 未修改 OpenAPI、生成物、后端、鉴权模型、secret 或企微凭据边界。

## Terra 调度记录

- 先后三次以 `gpt-5.6-terra` / `max` 在独立 clean worktree 调度 UI
  实现或测试语料扩充；三次均在分析阶段长时间无文件或 commit
  产出，由 Sol 中断。
- Terra 未修改任何路径，未产生可集成 head，因此不伪造 hash
  manifest，也不将 Sol 产出记为 Terra 收据。
- Sol 在已冻结合同内接管，记为一次 `infra_induced` 编排修正。

## 本地验证

- 固定 Node `24.18.0` / npm `11.12.1`。
- Orval 连续生成两次无 diff。
- ESLint、strict TypeScript、Vitest `81` 个测试、Vite build、
  `npm audit --audit-level=high`：PASS（0 个已知高危漏洞）。
- 全量 Go/repo-contract/gitleaks 与 GitHub CI 证据在本片完成时绑定
  ledger 和 PR，不在此预填 PASS。

## correction

- `slice_induced=2`：首次 focused 验证发现 transport 类型参数被
  核心 lint 误判为未使用，并暴露 mutation union 缩窄不精确；改为
  已使用的生成 wrapper 类型与 create/rename 独立结果。后续自审将
  401 callback 稳定化时首次放在早返回之后，在提交前移到所有
  早返回之前，保持 React hook 顺序。
- `infra_induced=1`：Terra Max 三次 clean 调度均无工具产出，Sol 在
  同一冻结范围内接管。
- `scope_induced=0`。
- `verification_induced=1`：预读 P2-18 时首次使用不存在的 shell glob，
  zsh 在读取前 fail-closed；改为对存在目录执行 `rg`，未修改文件。

## 外部门

- staging 浏览器 stages CRUD：`PENDING_EXTERNAL_GATE`。
- staging/生产部署、生产数据库、live migration、真实企微：
  `NOT EXECUTED`。

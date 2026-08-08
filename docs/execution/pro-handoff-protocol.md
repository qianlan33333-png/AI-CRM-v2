# ChatGPT Pro handoff protocol

## Input

每次从已验收 `main` 精确 SHA 生成最小源码包。任务清单必须列出 base SHA、
ADR、冻结接口、允许/禁止路径、必须测试、期望失败/成功和未授权外部门。

打包前：

1. 只选任务所需的 tracked files。
2. 拒绝绝对路径、`..`、`.git`、`.env*`、依赖、构建/缓存、数据库、日志、
   浏览器状态和真实数据。
3. 运行固定的 gitleaks 8.30.1 与仓库自带敏感路径扫描。
4. 运行 ZIP 完整性/路径穿越检查，记录文件清单、字节数和 SHA-256。

## Conversation

- 一个 Slice 新建一个对话；同片修正继续原对话。
- 先要求 Pro 复核输入 SHA，再开始工作。
- 不在生成中催促或重复任务；约每 10 分钟只读检查。
- 仅当连续三次、累计至少 30 分钟无任何输出且页面确认停止或报错时，才
  从保存 URL 恢复或发送一次继续指令。
- 保存对话 URL 到 Slice 台账。登录、验证码、Passkey 或二次验证交由用户。

## Output

Pro 必须交付基于 base SHA 的 unified patch、变更文件 ZIP、报告、文件清单、
测试日志及全部 SHA-256。未运行写 `NOT EXECUTED`；不允许 Git/GitHub、
部署、live migration、真实外部调用、凭据访问、架构变更或未经授权依赖。

## Codex acceptance

Codex 在精确 base SHA 的隔离 worktree 验证 ZIP 安全、哈希、白名单、依赖、
生成 no-diff、测试和安全边界。失败时给出日志、文件位置和正确契约；全绿
后才创建中文 PR、等待 CI、squash merge，并记录 merge SHA。

# P2-12 Terra Max UI 收据

- task：`/root/p2_12_web_shell_ui`
- executor：`gpt-5.6-terra` / `max`
- frozen base：`f459f2d2db549c65c0fbbc5876a810a4ae7e32eb`
- delegated head：`8bf3f1b2011276ee8ca150476995ad6ebbf1881f`
- allowed paths：`web/src/main.tsx`、`web/src/main.test.tsx`、
  `web/src/shell.css`
- correction：`slice_induced=1`、`verification_induced=1`、
  `infra_induced=0`、`scope_induced=0`
- Git 权限：仅本地 commit；未 push、未创建 PR、未 merge

## Manifest

规范格式为每行 `MODE SP BYTES SP SHA256 SP PATH LF`：

```text
100644 6732 5fbf5a75cc0e4684269938e753c9f0102998284c18a4ba146e331d0985eae73f web/src/main.test.tsx
100644 7332 ce8aedeec812313b6fa83896daad4619c8c2a5088915d3bc728f15da3061fa5f web/src/main.tsx
100644 4773 f6334efe23684e39b1987ebd3388aded29703c4c6c7a83303f76a287dbc6c25d web/src/shell.css
```

- manifest SHA-256：
  `c8b554485d9f344e61c83148246a2795d15a073129849fef233d58dc2cb344ab`
- `git diff BASE..HEAD --binary` SHA-256：
  `f4680bdc7d8087bb943f58629193f560394a40c4d9cdeb56fbb6d961232cd1e7`

## Sol 独立复核与集成

- `HEAD^` 精确等于 frozen base，`BASE..HEAD` 只有三条白名单路径，Terra
  worktree clean；Sol 从 Git object 独立重算 mode、bytes、文件 SHA-256、
  规范 manifest 与 binary diff，结果与上述最终收据一致。
- 初稿曾用 `@ts-expect-error` 处理 CSS import，Sol 拒绝永久抑制；Terra
  amend 为 `vite/client` 类型引用后，固定 Node/npm 环境下 Web CI 全绿。
- 首次 manifest 使用 TAB 分隔，Sol 复算发现与冻结格式不符；最终以单空格
  收据为准。两项均在同一 verification correction 周期内闭环，不伪造首次
  收据为已通过。
- Sol 集成后重新执行完整 Web CI、连续两次 Orval 无 diff、全量 Go CI、
  repo-contract 正负例与安全扫描；最终状态记录在 ledger 与 PR。
- Sol 首次完整 Web CI 因 PATH 中 Node 24 自带 npm 11.16 优先于冻结 npm
  11.12 而被版本门拒绝；调整 PATH 顺序后重跑通过，记为本片另一次
  `verification_induced`，未放宽版本门。
- 隔离 PG 首次 `createdb` 漏传固定测试密码而进入交互提示，Sol 立即取消并
  以 `PGPASSWORD=postgres` 重试；首次 `make ci-go` 又因 query-plan head
  使用缩写 SHA 被精确 SHA 门拒绝，改为完整 40 位 SHA 后从头重跑全绿。
  两次均记 `verification_induced`，未改数据库、query-plan 或 CI 合同。

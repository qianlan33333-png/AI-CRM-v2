# P2-13 Terra Max 登录与权限 UI 收据

- task：`/root/p2_13_auth_ui`
- executor：`gpt-5.6-terra` / `max`
- frozen base：`1bd9766b4758d89dbc6e02b14977eb26f204b086`
- delegated head：`adb6ad7092df149e171563cd527f29125688a27e`
- allowed paths：`web/src/auth-ui.tsx`、`web/src/auth-ui.test.tsx`、
  `web/src/shell.css`
- correction：`slice_induced=1`、`infra_induced=1`、
  `verification_induced=1`、`scope_induced=0`
- Git 权限：仅本地 commit；未 push、未创建 PR、未 merge

## Manifest

规范格式为每行 `MODE SP BYTES SP SHA256 SP PATH LF`：

```text
100644 8170 a45b33d9bcc123347b5b6878b16c79fbc8b390798fc423efb82fd0b9582b1ddd web/src/auth-ui.test.tsx
100644 6231 cd44efe0eb1216921df18f4f508088a12004a97eb4f148698e46b776f9b6cde6 web/src/auth-ui.tsx
100644 8169 b9886ea9ba79100b7572e8c5f184172a3c9d118a2793e89fa7b4c5d6cb67201a web/src/shell.css
```

- manifest SHA-256：
  `c767e89716f502a27630c4fbf22e068597b5b12b0c1db164852bcb1bf41a763f`
- `git diff BASE..HEAD --binary` SHA-256：
  `5a1fb883e7beffe24fc7f7ec6f208a314215e8912ff517fe578585ad199c9a40`

## Sol 独立复核与集成

- `HEAD^` 精确等于 frozen base，`BASE..HEAD` 只有三条白名单路径，
  Terra worktree clean；Sol 从 Git object 独立复算 mode、bytes、文件
  SHA-256、规范 manifest 与 binary diff，结果与最终收据一致。
- 首稿有三处 core ESLint 抑制，Sol 拒绝依赖抑制通过门禁；
  Terra amend 为 React 内置事件类型后重跑 lint、strict typecheck、
  15 条 focused test、build 与 Prettier 全绿，不新增 correction。
- 组件未导入生成 client，不 fetch、不读 cookie/storage、不推导
  role/capability；权限、CSRF、logout 与 60 秒缓存均由 Sol 核心集成。
- Sol 集成后重跑完整 Web CI、连续两次 Orval 无 diff、全量 Go CI、
  repo-contract 正负例与安全扫描；最终状态记录在 ledger 与 PR。

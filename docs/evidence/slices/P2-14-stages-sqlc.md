# P2-14 Terra Max stages sqlc 收据

- task：`/root/p2_14_stages_sqlc`
- executor：`gpt-5.6-terra` / `max`
- frozen base：`f2c620193ab4fa8b2e686a05ddd2f70b20113daf`
- delegated head：`bf0428aa92207f316511e099407ba581ea5d4ecc`
- allowed path：`internal/contact/store/queries/stages.sql`
- correction：`slice_induced=0`、`infra_induced=0`、
  `scope_induced=0`、`verification_induced=0`
- Git 权限：仅本地 commit；未 push、未创建 PR、未 merge

## Manifest

规范格式为每行 `MODE SP BYTES SP SHA256 SP PATH LF`：

```text
100644 430 0a0ecd3338cabecf50261d114a6036727e29dd87de01019ff7513f8b002162ca internal/contact/store/queries/stages.sql
```

- manifest SHA-256：
  `2280623933483d2df3c31637151d75272a0c9904b9cc962f3789d2e7abd2d854`
- `git diff BASE..HEAD --binary` SHA-256：
  `d9f8f1e647e2bc39ca3dbef86486199e75a575ee077417de779924ce0c6ef613`

## Sol 独立复核与集成

- `HEAD^` 精确等于 frozen base，`BASE..HEAD` 只有唯一白名单路径，
  Terra worktree clean；Sol 从 Git object 独立复算 mode、bytes、文件
  SHA-256、规范 manifest 与 binary diff，结果与收据一致。
- 三条查询精确实现 `ListStages`、`InsertStage`、`RenameStage`；
  列表按 `sort_order, id` 排序，无 delete/reorder/config update、动态 SQL、
  `COUNT(*)`、OFFSET 或跨表访问。
- Terra 执行 `sqlc vet` 前后文件清单与 hash 一致，未代生成物；
  Sol 集成后使用冻结 sqlc 版本生成全部 contact store 代码并完成门禁。

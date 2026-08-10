# P2-10 Terra Max 测试语料收据

- task：`/root/p2_10_rbac_test_corpus`
- executor：`gpt-5.6-terra` / `max`
- frozen base：`9400330c9a63baa313bd5969a5c51ce61bfc67c8`
- delegated head：`d3800270be445c41054b52d8eb652b4068f42d28`
- allowed path：`internal/auth/app/policy_test.go`
- correction_count：`0`
- Git 权限：仅本地 commit；未 push、未创建 PR、未 merge

## Manifest

```text
100644 11753 65e4b9fc087b622da68177b1d4b9eb3a6652c8e9841b431b1863c327bdf66b4a internal/auth/app/policy_test.go
```

- manifest SHA-256：`c7dbc1474f6ce79a3b94cca9320bf35b5120af9f8526a1c7aa0950f2f9311bf5`
- `git diff BASE..HEAD --binary` SHA-256：
  `4b93350facf108ad37cc92c90170ed3d452a1270e3e0158a360fe99d97d35308`

## Sol 独立复核

- `HEAD^` 精确等于 frozen base，`BASE..HEAD` 仅包含白名单文件。
- Sol 从 Git object 独立重算 bytes、文件 SHA-256、manifest SHA-256 与 binary
  diff SHA-256，均与 Terra 收据一致。
- Sol 独立执行
  `GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go test -race -count=1 ./internal/auth/app`
  通过。
- 测试覆盖三角色 × 九个 capability、未知 capability、无效 principal、
  nil/cancelled context、owner scope 和 authorization context 正负路径。

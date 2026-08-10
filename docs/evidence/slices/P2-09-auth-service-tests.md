# P2-09 Terra Max 认证服务测试收据

- 委派任务：`p2_09_auth_service_test_corpus`
- 执行器：`gpt-5.6-terra` / reasoning `max`
- 允许路径：`internal/auth/app/service_test.go`
- Base：`9a4bb609dd7adf1269a4ee4436bc130d06ba42f0`
- Head：`f6bce0ad974c9ea06a70efa91e3dd013b6f0370f`
- correction_count：`0`

## 文件 manifest

```text
100644 23026 5509566ace3f86f2a8f5830fadf9dce29e8cc1d6458a7be4ec30b059f66f507f internal/auth/app/service_test.go
```

- manifest SHA-256：`16fc535aa0ecc9f799dc240efaef9530f4fb8f953e248e073f250c51724bab44`
- `git diff BASE..HEAD --binary` SHA-256：
  `b484d991aec9cbf209d569a1de839e2566dc4a2ef341a39135ebce20737b9318`

## Sol 独立复核

- 确认 head 的直接父提交等于 base，worktree clean，变更仅含允许路径。
- 从 git object 重算 mode/bytes/file hash、manifest hash 和 binary diff hash，
  与 Terra 收据逐项一致。
- 在 Terra worktree 独立重放
  `go test -race -count=1 ./internal/auth/app`：PASS。
- 测试提交经 Sol cherry-pick 进入切片分支；Terra 未 push、未建 PR、未合并。

# P2-04 Terra Max 队列策略测试语料收据

- task_id: `p2_04_queue_policy_test_corpus`
- executor: `gpt-5.6-terra`
- reasoning: `max`
- base_sha: `4810d5b1ce00263390d2ddb8f674e9c9d48eb9bf`
- delegated_head_sha: `6eeb35e3ca7b2e5cd62597035bd5123b40e67a0e`
- integrated_commit_sha: `4a071e6`（本地 cherry-pick；最终以 PR head 为准）
- allowed_path: `internal/platform/jobqueue/queue_policy_test.go`
- actual_paths: `internal/platform/jobqueue/queue_policy_test.go`
- correction_count: `0`
- test: `go test -race ./internal/platform/jobqueue` → PASS
- Sol independent replay: `go test -race -count=1 ./internal/platform/jobqueue` → PASS
- diff_sha256: `b89f25b9d846719b79be6526cc31dbc66828674e73cfc186350a5e45b34b5c0f`
- file_manifest_sha256: `c3034ec89152fd87873e1a13383db0e096bff8a236e1e617e6e14c68ae52638c`

```text
100644 10870 12f2041543ad7da6af1d068067184e50d20c36732ee7e6a39423cc0da7053cf1 internal/platform/jobqueue/queue_policy_test.go
```

Sol 已独立核对直接父子关系、唯一修改路径、mode/bytes/file SHA、manifest SHA、
canonical binary diff SHA 与 `git diff --check`，并逐项审查新增测试只扩充已
冻结的六队列白名单、并发预算、worker 注册、critical timeout 与显式 options
规则，没有修改生产实现、中央合同、migration 或黑盒验收夹具。

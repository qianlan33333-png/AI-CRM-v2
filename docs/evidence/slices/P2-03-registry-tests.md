# P2-03 Terra Max registry 测试语料收据

- task_id: `p2_03_registry_test_corpus`
- executor: `gpt-5.6-terra`
- reasoning: `max`
- base_sha: `8500031bb0b59426256baae763f0b5b098786960`
- delegated_head_sha: `a6d69cea2a16e03e005552df29fc98092d27ef5b`
- integrated_commit_sha: `904d629`（本地 cherry-pick；最终以 PR head 为准）
- allowed_path: `internal/config/registry_test.go`
- actual_paths: `internal/config/registry_test.go`
- correction_count: `0`
- test: `go test -race ./internal/config` → PASS
- Sol independent replay: `go test -race -count=1 ./internal/config` → PASS
- diff_sha256: `1b64f90727c5cb9358b96e4542d3f16b12d70398491aa034f310bac0112ca81b`
- file_manifest_sha256: `aac9123bb413a9c1389c2aac8007f8eecd4ba9cc9b0e1f6f85930f74eb994628`

```text
100644 10356 fab5be640b3417346a4b3b0e5a13be288cc38c85bdb3399f2459a1c300235fdb internal/config/registry_test.go
```

Sol 已独立核对祖先关系、唯一修改路径、mode/bytes/file SHA、manifest SHA、
canonical binary diff SHA 与 `git diff --check`，并逐项审查新增测试只扩充冻结规则的
正负边界，没有修改 registry、schema、port、migration 或生产实现。

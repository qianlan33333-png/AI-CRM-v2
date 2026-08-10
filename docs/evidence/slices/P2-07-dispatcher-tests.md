# P2-07 Terra Max dispatcher 测试语料收据

- task_id: `p2_07_dispatcher_test_corpus`
- executor: `gpt-5.6-terra`
- reasoning: `max`
- base_sha: `2f24e531a1525e8ceea43c34c1945380a2c79c7f`
- delegated_head_sha: `eae4a7d2f7413afd519f5cc45fdff94736008533`
- integrated_commit_sha: `1619f14`（本地 cherry-pick；最终以 PR head 为准）
- allowed_path: `internal/events/dispatcher/dispatcher_test.go`
- actual_paths: `internal/events/dispatcher/dispatcher_test.go`
- correction_count: `0`
- test: `go test -race ./internal/events/dispatcher` → PASS
- Sol independent replay: `go test -race -count=1 ./internal/events/... ./cmd/aicrm` → PASS
- diff_sha256: `c9f7b84ba93399e70c57493a6e463464d04008211e5e2b0e63456de8cd41038c`
- file_manifest_sha256: `47beba230be48db7374364e82956ff729a116160bf6e74415e7f839e70024431`

```text
100644 7716 a712eae9c1fa339cc2233a847b45f6a3473d1de8162d8eddd8e4ad8c627832b1 internal/events/dispatcher/dispatcher_test.go
```

Sol 独立核对 base/head 直接父子关系、唯一修改路径、mode/bytes/file SHA、
manifest SHA、canonical binary diff SHA 与 `git diff --check`，并逐项审查新增测试
只扩充已冻结的 dispatcher、router、worker 与 River insert options 合同，没有修改
生产实现、迁移、公共 port 或黑盒验收夹具。

集成后架构 lint 发现 Terra 语料直接引用 `river.QueueDefault`。Sol 将该负例
改为字面 `"default"`，这是 `verification_induced=1`；Terra 原始收据保持不变，
最终集成文件另由 repo-contract 收据锁定。

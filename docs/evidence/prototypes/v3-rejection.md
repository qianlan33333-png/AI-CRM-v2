# ChatGPT Pro v3 prototype rejection

状态：`REJECTED_AS_BASELINE`

v3 只保留为研究和失败证据，禁止整体导入本仓库。

## 来源与工件

- 原始对话：https://chatgpt.com/c/6a76ab1a-f208-83ee-8ca8-6217a71a91d9
- v3 对话：https://chatgpt.com/c/6a76cb4f-2b24-83ee-8ccf-0149a27083f4
- ZIP：380,915 bytes；SHA-256
  `48c3e3d8a244dff256b746ba9cdc359f8128d5a59eb30eba637aea0b628ed662`
- Patch：1,225,857 bytes；SHA-256
  `35164ef7825e4a98a471449562bd0931f7ceb5a4fd12db75041ff23f7eb9e00b`

二进制工件只保留在本地 handoff 目录，不进入 Git。

## 独立拒收依据

- 缺少可复现的 `go.sum` 与 `web/package-lock.json`。
- oapi-codegen、sqlc、Orval 官方生成物不完整，无法通过生成一致性门。
- `make check` 因未获证据的功能矩阵 disposition 永久失败。
- Go 代码在官方生成后出现大量类型和符号编译错误，集成测试未运行。
- 大量前端页面是通用表格/JSON 展示，不能证明旧行为等价。
- 旧 OneID DDL 与渠道中立 OneID 冲突；补丁式 migration 不适合全新仓库。
- 迁移工具存在数值静默截断、全表入内存和 source-cut 不稳定问题。

可复用的仅是测试思路和失败样本；任何代码必须在新的精确基线下重新
拆片、生成、测试和验收。

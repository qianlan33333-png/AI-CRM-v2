# P0 阶段 closeout

状态：`APPLICATION_GREEN_BASELINE`

证据时间：2026-08-09 14:44:17 +08:00

权威 main：`98f0ac4813aff623a9a7c0790e57c8c1993a8fd3`

## 结论

P0 已建立可运行、可生成、可机械检查的应用绿色基线；这不是产品功能完成、staging
验收或生产验证。最终代码已经提交、推送并 squash 合并到私有仓库 `main`，没有部署、
没有真实数据迁移、没有真实企微调用，也没有修改线上配置。

P0 后半段已按用户最新决策切换为单 Sol 垂直 Slice：同一负责人完成架构裁决、实现、
作者测试、中央门修正、Git/PR、合并与 main CI，不再为小实现创建 Terra 中间合同 PR。
后续 P1 只有互不依赖、规模足够的事实盘点才并行；迁移与对账仍必须由独立 Agent 复核。

## 垂直 Slice

| Slice | 可观察行为 | PR | 有效 merge SHA |
|---|---|---|---|
| P0-S01 | `--role=api|worker|all` 生命周期骨架 | [#8](https://github.com/qianlan33333-png/AI-CRM-v2/pull/8) | `eb998ebe78de6d0521009e57d9f4b184026b0158` |
| P0-S02 | strict `/healthz` handler | [#36](https://github.com/qianlan33333-png/AI-CRM-v2/pull/36)；portable mode 修正 [#38](https://github.com/qianlan33333-png/AI-CRM-v2/pull/38) | `1ad85c1068409dd51322fe9ce46156422063297c` |
| P0-S03 | sqlc Ping adapter 与 PG 查询 | [#41](https://github.com/qianlan33333-png/AI-CRM-v2/pull/41) | `a5c11d11d6f55903a74fcac6e4288544dc8f4be1` |
| P0-S04 | River Runtime 与官方 migration adapter | [#44](https://github.com/qianlan33333-png/AI-CRM-v2/pull/44) | `ad21879f916df5d548aacbeb4fdfb1fdf214b09e` |
| P0-G02 | P0 单 Sol 垂直治理 | [#46](https://github.com/qianlan33333-png/AI-CRM-v2/pull/46) | `94ec1016954c1a14c62ede42d299599b444f254d` |
| P0-S05 | React/Vite 应用壳 | [#48](https://github.com/qianlan33333-png/AI-CRM-v2/pull/48) | `098fd38065b0c8b2ab75e251a00ce5b6f26f8e54` |
| P0-S06 | Orval health client 生成闭环 | [#50](https://github.com/qianlan33333-png/AI-CRM-v2/pull/50) | `6a654baf71149dfa55900d9dd0a025c12725f8cc` |
| P0-S07 | 跨模块 import lint | [#52](https://github.com/qianlan33333-png/AI-CRM-v2/pull/52) | `b96e9f30c8d1aef24f8b8e739e987b919fc37d77` |
| P0-S08 | 表与企微操作归属 lint | [#54](https://github.com/qianlan33333-png/AI-CRM-v2/pull/54) | `59385b5a1e3a4922e0260df1cfd2af9761dc5d7e` |
| P0-S09 | env、手拼 SQL、业务 timer lint | [#56](https://github.com/qianlan33333-png/AI-CRM-v2/pull/56) | `840befb5c85f5694512fb190886e5106f2215262` |
| P0-S10 | 空 contract-replay 结构门 | [#58](https://github.com/qianlan33333-png/AI-CRM-v2/pull/58) | `98f0ac4813aff623a9a7c0790e57c8c1993a8fd3` |

P0-S03 之前的合同/门禁 PR（#16、#18、#20、#40）及 P0-S04 的合同/门禁 PR
（#22、#25、#26、#28、#30、#32、#43）仍是 Git 历史中的 point-in-time 证据；
上表记录最终可观察行为的实现入口，不抹除这些历史收据。

## 最终累计 main 验收

| 门 | Actions 证据 | 结果 |
|---|---|---|
| Go + Web + PostgreSQL | [run 31299549088](https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31299549088) | Go 1.26.5、race、vet、build、govulncheck、生成二次无 diff；Node 24.18.0/npm 11.12.1、Orval、lint、typecheck、Vitest、build、audit；PostgreSQL 16.14、Goose up/down/up、sqlc 查询和 River migration 均成功 |
| 仓库合同 | [run 31299549094](https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31299549094) | repo-contract 与永久负例成功 |
| 安全 | [run 31299549099](https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31299549099) | gitleaks 全历史、敏感路径和 bundle 安全负例成功 |

P0-S10 的 `PASS` 仅表示 canonical 空 manifest 的 synthetic structure gate；非空 case
会明确失败，尚未打向 staging，也没有逐字段业务 diff 或 HTML 报告。

## 未验证与权限边界

- G1 旧行为/API/页面/迁移映射尚未盘点并获得人工签字；P1 尚未开始。
- staging 登录、真实企微、真实问卷/MCP、性能 S 档、生产回放与真实数据迁移均未执行。
- 没有提交部署工作流、Environment、Secrets 或生产配置；当前状态为已合并、未部署。
- GitHub Free 无法给私有 `main` 强制 Ruleset；当前只有“CI 全绿才 squash”的流程约束，
  不能声称分支已受 GitHub 强制保护。
- 旧仓及父目录 `handoffs/` 的历史工作树/证据不在新仓源码范围内，未被清理或覆盖。

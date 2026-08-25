# 前端能力范围

本清单只用于新 TypeScript 前端的能力分母；不修改 `docs/feature-matrix.csv` 中的历史后端迁移事实。

## `excluded_duplicate_page`

- `LEGACY-S05-059`
- `LEGACY-S06-006`, `LEGACY-S06-017`
- `LEGACY-S07-001` 至 `LEGACY-S07-042`
- `LEGACY-S07-093`, `LEGACY-S07-106`, `LEGACY-S07-121`

以上 48 项是旧页面打开、模板渲染或导航的重复计数。对应 CRUD、查询、导出、审核与 receipt 仍由实际 OpenAPI action 计入。

## `presentation_only`

- `LEGACY-S05-011`, `LEGACY-S05-019`, `LEGACY-S05-028`, `LEGACY-S05-036`
- `LEGACY-S06-005`, `LEGACY-S06-007`

以上 6 项不产生服务器请求或业务事实。新页面可保留安全的浏览器展示行为，但不得计为 API 接入或真实业务成功。

## 规则

其余页面动作只能标为 `real`（当前 OpenAPI 已由 Adapter 调用）或 `backend_blocked`（不存在或语义不等价）。禁止以 Mock、Seed、定时跳转或成功 toast 填补缺口。

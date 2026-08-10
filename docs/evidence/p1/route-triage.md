# G1 路由 A/B/C 分档与 G1-D02 签字

## 结论

基于用户授权的生产环境只读取证与冻结 UI 矩阵，781 条 authority 路由已全部完成 G1-D02 规则签字：

| 分区 | A：MIGRATE | B：DEFERRED_POST_LAUNCH | C：NOT_MIGRATED | 合计 |
|---|---:|---:|---:|---:|
| S02 contact/auth/admin | 99 | 56 | 1 | 156 |
| S03 WeCom/segment/outbound | 106 | 76 | 2 | 184 |
| S04 上层域 | 296 | 136 | 9 | 441 |
| **总计** | **501** | **268** | **12** | **781** |

规范化逐路由表见 `docs/evidence/p1/route-triage.csv`。A 501 条保留 1:1 旧业务语义，B 268 条上线后重评且不是废弃，C 12 条继续由 G1-D01 锨定不迁。全部 `human_signoff=APPROVED`。10 个核心新 OpenAPI 的批准不代替后续各域的 operation 冻结。

## 判据

按 v2 附录 B，并采用保守优先级：

1. 生产窗口内存在真实调用记录：A。
2. 零调用但 feature matrix 存在页面或 API 引用：A，避免误删低频 UI 能力。
3. 零调用且 authority 明示 retired/blocked/fixture：C。
4. 其余零调用、无 UI 引用：B。

339 条路由有生产流量，另有 162 条因 UI 引用但窗口内零流量进入 A；268 条进入 B；12 条进入 C。`/api/h5/wechat-pay/{path:path}` 虽带 blocked 标记，但窗口内有 1 次调用，因此升级为 A，不在 C 中批量确认。

## 生产证据窗口与最小化

- 读取窗口：`2026-07-11T00:00:00+08:00` 至 `2026-08-10T10:15:00+08:00`。
- 主应用在 `2026-07-25` 后提供结构化 route summary：30,772 条，全部逐 `method + route + route_name` 精确匹配 authority。
- 主应用前半窗口与独立回调入口使用 journald 访问行：52,454 条；去除查询参数后有 27,840 条匹配 authority，24,614 条为静态资源、扫描、未注册路径等，不进入 781 路由频次。
- 合计读取 83,226 条最小请求元数据，其中 58,612 条原始事件至少匹配一个 authority 路由。
- 10 组早期动态路径共 379 次存在同形路由歧义；相关候选全部保守升 A，CSV 的 `ambiguous_access_count` 明示该部分，不能跨行求和还原原始总量。
- 未收集或落库 IP、请求体、响应体、cookie、authorization、查询参数或企微签名；未部署录制中间件。

只读聚合输入收据：

- 结构化 route summary：`f0e148a404be8f375093f73c9239b9abc79c1045a6a9c38cde909f1acc754990`
- 主应用早期去查询参数访问聚合：`653a08b285d4a070fee4d2d51df9d6b89abe7863acc82e777b5c073e3fcde403`
- 独立回调入口去查询参数访问聚合：`f638b711b1c08790613bbf0e6662882a3d3bcf75ef2c7e00f94a6913d9560e2d`
- G1-D01 后 781 行分档表：`875596da33d316c31bff9a6103725affa58c44be399ec98239d9e294c34c069b`

## G1-D02 终态

1. A 档 501 条全部 `MIGRATE/APPROVED`。
2. B 档 268 条全部 `DEFERRED_POST_LAUNCH/APPROVED`，不得改写为 `NOT_MIGRATED`。
3. C 档 12 条全部 `NOT_MIGRATED/APPROVED`；任何未来恢复都必须走新的人工裁决。
4. 实际数量 781 比旧指令 758 多 23 条，已按实际 tier 签字，未回退或遗漏。

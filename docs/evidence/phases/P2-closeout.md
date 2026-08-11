# P2 平台层收口

## 结论

P2 活跃切片 `P2-00`、`P2-01R`、`P2-02` 至 `P2-18` 已全部合并；原
`P2-01` 保持 `SUPERSEDED_BY_RESCOPE`，未把 contact 包伪装为 UoW
验收。P2 最终 main SHA：

`0c4c1bd697631d6242fdf838d17303970635144f`

该 SHA 的 application/Web、repo-contract 与 secret scan 分别为
`31489713217`、`31489713213`、`31489713186`，全部 PASS。

## 完成内容

- acceptance fixture、动态安全 DSN、工具缺失 fail-closed 与 shell 变量
  shadow lint。
- transaction-bound UoW、强类型启动配置、settings/secret 边界。
- River 六队列、唯一 scheduler、事务 event append、dispatcher crash
  recovery。
- request ID、错误码、panic recovery、结构化日志、session、RBAC、并发
  预算与中间件顺序。
- React shell、登录/权限 UI、60 秒缓存。
- stages store/service/event/OpenAPI/handler/snapshot/Orval UI。
- S/M/L 分档生成、Compose 与 staging apply 边界。

## 自行决策与 correction

- 按用户授权将 infra/scope/verification 修正自行闭环并逐片留证；只有
  `slice_induced >= 3` 触发停报，P2 未触发该门。
- 为真实 PG 验收新增 `acceptance_fixtures` 专用 schema，未放宽 public
  ownership 门。
- 为第三方可达漏洞升级根依赖，并将 Orval 本地缺失从静默跳过改为显式
 失败。
- P2-01 因 scope 欠定义重划为 P2-01R；P2-00 因外部依赖暂停后恢复，
  两者未混写或重置 correction。
- River 固定六队列、`inbox_events`、`outcome_unknown` 与 P4
  `event_deliveries` 延后已同步设计文档。

## backlog

普通旧行为缺陷的唯一出口仍为 `docs/backlog/post-launch.md`。P2 没有把
安全、数据损坏、铁律、ADR 或硬性能门写入 backlog 规避。

## 未执行项

- 真实企微 OAuth、企微凭据、真实回调与发送：`PENDING_EXTERNAL_GATE`。
- 生产数据库、live migration、生产部署：`PENDING_EXTERNAL_GATE`。
- G2 浏览器登录、Stages CRUD 与人工签字：`PENDING_EXTERNAL_GATE`。

P2 代码出口完成不等于 G2 人工出口完成；后者见
`docs/evidence/g2/test-server-deployment.md`。

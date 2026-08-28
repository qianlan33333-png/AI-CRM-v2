# Cycle observation history — candidate receipt

集成更新：PR587已在最终候选c2bf/Full33215882004通过后合并；exact-main e9a0e449/Full33217605005通过。本分支已串行整合该验证基线，保留Contact135及全部CLI参数/路由，136编号连续；migration-mapping现在PASS（316rows/217physical/3312columns/pending0），OpenAPI与Go生成两遍通过。尚未表示本批候选Nightly通过或正式136已部署。

本批仅21条周期指标、18条周期引用观察；不恢复周期任务，不创建当前run/snapshot关系。开发与隔离演练基于已验证5f4b9f48；集成到下一正式基线及最终136序号须等待PR587 exact-main绿灯。

本地候选 `88abfceebd5d6fa0fac64289c789a36be3035120`：

- Go领域/导入/HTTP测试、race/vet、OpenAPI契约、生成物两遍、ownership/source-policy/architecture检查通过。
- 完整Web typecheck、build（56页面+4bundles）、npm test（390/0及全部历史专项）通过。
- migration-mapping检查因并行worktree暂缺待合并135而报编号gap；不得跳过此门禁，更新已验证基线后重跑。

真实V2隔离演练：既有network=none容器 `aicrm-pre-automation-combined-dcf28fe` 内新增 `aicrm_cycle136`。只读提取已有 `aicrm_contact135` 的schema/goose及正式134中的39条加密archive/对应ingress与原始seal；没有复制业务客户/身份。

- 39条源/目标COPY数量和SHA相同；原4库OID/schema不变。
- schema136与 `TestCycleObservationHistoryPostgresRoundTripRollback` 通过（0.02s）。
- 首次导入39；重放新增0、重放39；两次对账均selected/receipt/imported/verified=39、archive/quarantine=0。
- 两次seal：`02c643019f9b64a94129a91a736c59aa710df91737a623e99011253f9436ec03`。
- 参考数据快照不变，runtime=0，正式schema134/startup/system-id不变，演练容器已恢复exited/none。

证据目录：`/data/aicrm/cycle136-rehearsal/evidence-88abfceebd5d6fa0fac64289c789a36be3035120`。
准备日志SHA `8c37704c6d8eccfb7c62eb849c3385fd0b24a03b09e4be2b3283dc377a54fc60`；
演练日志SHA `f979e1a42d80a63e5a08ed296e8f8674b8b2b74514df6dbc5404dae1d69818da`；
两次JSON证据SHA `ecbdae34727c9d371331c66e3ffbd21744c8ba9cada343a9c13b04bf938026d0` / `427ca504cfed6063f812ee526e59e67ab9655f952c30e96df045177d2869a94c`。

此为隔离候选证据，不是正式136已部署；不改变严格Excel完成数，不代表当前HXC漏斗、Sidebar实际登录或Provider外部效果验收。

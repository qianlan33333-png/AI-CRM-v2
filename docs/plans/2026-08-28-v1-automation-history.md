# 自动化四表历史包（119，暂不部署）

前序118已通过PR575候选Full33153339004，合并为dcf28fecb3d2e30ea1046e4c76fb8ecc87602eca；合并后exact-main33154862936已成功，TARGET_SHA与TESTED_SHA均核对为dcf28fecb3d2e30ea1046e4c76fb8ecc87602eca，commit compatibility状态成功。当前已对齐该代码，可进入119集中PR；不跳号部署。

冻结V2封存归档run v1-full-archive-20260827实际只读预检：SOP16、agent config12、prompt registry6、agents6，共40历史候选，0隔离，无脱敏占位输入。V1不连接、不修改。

采用Automation-owned四张typed只读历史表，不覆盖现有automation_agent_configurations，不恢复执行/发布/调度。SourceID及四种workflow引用仅为历史source refs；prompt/JSON/actor值只以摘要与封存来源保留。旧enabled/status/版本及原始日期仅供观察，空字符串和不确定时间不猜转换。

1. 私有源适配器已经预检通过；主代理串行定义Port、119及所有权。
2. 叶代理仅实现Automation app/store/queries与测试；其他叶代理仅实现私有import/journal/reconcile，不改CLI/共享文件/生成代码。
3. 主代理串行CLI/API/OpenAPI生成和只读UI，使用当前鉴权；真实读取失败关闭，不Mock回退。
4. 40条首次导入、全量重放、两次typed逐行对账，current automation/event/jobs/effects不增。代码CI→最终PR候选Full Nightly→绿灯合并→exact-main Full Nightly→仅V2部署。

这只完成旧配置的正式历史落表，不等同当前Agent执行、真实群发或Excel中所有能力销项。V1零修改、现有流量零切换；最终用户人工测试后另行确认。

已完成隔离真实演练：40首次/40全量重放、双次verified40/40，quarantine0，seal577ebe17aa4f3afab6cb7d5773c0e39d3b3c8aa9307c5b1423f96d6cc0ccaffa一致；最终组合4a1e9430及Linux importer aca05f57bf9c4c5cab553cb2eec670163e897ac5ef2750d94231ff18152cd333再次重放及双对账通过。effects/event_log/river_job前后0|0|0，未连接V1。前端383通过/0失败；本次基线对齐未改变业务树，仅纳入前序Campaign文档。正式119、普通用户登录与真实业务外部效果均未验收。

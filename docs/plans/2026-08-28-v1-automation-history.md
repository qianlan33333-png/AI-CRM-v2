# 自动化四表历史包（119，暂不部署）

基线：已验证3825400。前序115–118顺序门禁全部通过后再集中PR集成；不跳号部署。

冻结V2封存归档run v1-full-archive-20260827实际只读预检：SOP16、agent config12、prompt registry6、agents6，共40历史候选，0隔离，无脱敏占位输入。V1不连接、不修改。

采用Automation-owned四张typed只读历史表，不覆盖现有automation_agent_configurations，不恢复执行/发布/调度。SourceID及四种workflow引用仅为历史source refs；prompt/JSON/actor值只以摘要与封存来源保留。旧enabled/status/版本及原始日期仅供观察，空字符串和不确定时间不猜转换。

1. 私有源适配器已经预检通过；主代理串行定义Port、119及所有权。
2. 叶代理仅实现Automation app/store/queries与测试；其他叶代理仅实现私有import/journal/reconcile，不改CLI/共享文件/生成代码。
3. 主代理串行CLI/API/OpenAPI生成和只读UI，使用当前鉴权；真实读取失败关闭，不Mock回退。
4. 40条首次导入、全量重放、两次typed逐行对账，current automation/event/jobs/effects不增。代码CI→最终PR候选Full Nightly→绿灯合并→exact-main Full Nightly→仅V2部署。

这只完成旧配置的正式历史落表，不等同当前Agent执行、真实群发或Excel中所有能力销项。V1零修改、现有流量零切换；最终用户人工测试后另行确认。

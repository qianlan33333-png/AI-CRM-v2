# V1 HXC chat-job history — minimal migration plan

目标：将 V2 冻结归档中153条 `public/automation_laohuang_chat_job` 按 HXC 领域保存并提供只读查询，不恢复旧任务、不回调、不发送。当前仅并行准备，不是已合并/部署。

来源已于2026-08-29从V2只读核验：153行、21列、PK=id，该源 domain-import receipts=0；未访问V1。旧mapping的ABSENT_AT_HEAD不能代替真实冻结manifest。

选择复用现有 HXC 历史 writer、journal、分页页面，不新建任务平台。归档单存缺读取入口；直接恢复旧job会改变外部执行，因此均不采用。源queue/member/send_record只保留source ID，不猜V2当前关联；确有已有可信crosswalk时再由主代理串行接入。私有身份、手机号、消息/会话/task标识、JSON payload、reply/error正文保留私有证据，不进入公开DTO。finished_at是NOTNULL text，保持原文、不猜时间解析。

1. 来源adapter与selector：精确21列、三HMAC、NULL/空串/原始JSON、signed ID/UTC微秒测试；从已验证origin/main隔离叶子开发，仅新私有目录，独立commit。
2. 主代理串行冻结HXC Port及下一空闲DDL（当前135/136尚未门禁闭环，禁止先定137）、真实SQLC、领域writer/store和导入/重放/双对账。每个source字段进入typed fact或私有证据摘要；历史非可执行。
3. 主代理接入最小admin GET列表/详情及OpenAPI，生成Orval后叶子复用既有HXC历史页面；验证鉴权、分页、空态/失败态、私有字段不泄漏、无Mock生产回退。
4. V2独立network=none scoped演练153首次导入、重放新增0、双对账同seal及无runtime副作用。不得更改旧归档或V1，避免新的全量卷。
5. 一份集中PR：包级验证→CI→最终候选SHA Full绿→合并→exact-main Full绿→正式备份/迁移/独立id-dev部署。不得因排队提前合并。

验收分开：typed history/账本、exact-main验证、V2部署人工可测、真实Provider效果。此包不替代810554代次投影迁移，也不关闭当前HXC漏斗或Sidebar实测缺口。七业务外部开关继续false；老入口不切流。

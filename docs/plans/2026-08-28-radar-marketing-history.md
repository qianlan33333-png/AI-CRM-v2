# Radar 点击与营销配置历史

全量迁移范围内补三表：radar_click_events 1735行；marketing_automation_configs 1行及question_rules 3行。候选已有V2只读预检；本分支从已验证e67c5b94创建，仅预留126，前序120–125未并入前不作为可部署版本。

Radar新增历史观察表，不写当前radar_link_events、不累计当前点击。V1 link_id仅是源引用；V2 RadarLinkID必须通过v1-domain-a1/public/radar_links已验证回执关联，缺失则NULL并保留源引用，不能猜同号外键。CustomerID只允许DM01已验证unionid映射，缺失则NULL。身份/原文摘要保持私有。

Automation新增营销配置及规则历史表，规则指向本包已导入历史配置；问卷/题目仍明确为V1源ID，旧status/is_active不启用自动化。仅提供Admin只读列表/详情和现有页面历史入口。

主代理串行Port、126 DDL、SQLC、OpenAPI/路由/生成、CLI与集成；子代理限定owner或私有导入路径。实际隔离导入、重放、双对账后，随集中PR完成最终候选Full与exact-main；再部署V2。V1零修改、不切流、不触发外部效果，不把历史读取当当前业务能力销项。

## 已完成隔离验证（不是正式部署）

V2既有network=none隔离库实际125→126；两域Store PostgreSQL往返/回滚通过。Radar1735首次导入、1735重放、零隔离；两次对账均1735且摘要 `f7063a2692a529752b4a804b8094a3a3738588780eda3c5d5b2c60d19f7058ce`。其中1703条关联6个已核验Radar链接、786条关联201个DM01客户，余下关联保持NULL。

营销配置1/规则3首次导入、4重放、零隔离；双对账摘要 `b35e06710b6b04899bf72d5a5d1bcea21d6d85cd0458d356ed026f729489f614`，3规则均正确关联历史配置。两域全过程effects/events/jobs保持0/0/0。

本地相关Go单测/race/vet、source-policy38、ownership、Web typecheck、383项e2e和10项专项DOM通过。前序120–125仍需统一集成，再验证最终候选Full、exact-main和正式部署；正常用户登录/人工测试入口仍单独验收。

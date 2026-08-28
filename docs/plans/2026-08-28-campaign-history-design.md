# V1 Campaign 五表历史最小正式落表

- 仅读取V2已封存的五张源表：Campaign segments 7,338、members 6,707；Broadcast plans 551、recipients 5,442、messages 5,442，合计25,480。源campaigns 6,382仅作父关系上下文，不再重复导入。
- Campaign-owned五张只读历史表，Schema118；不写当前Campaign、Segment membership、Outbound、WeCom、event或queue。
- 源campaign_segments中958条缺源campaign父，而原V1无此物理FK。保留合法源事实并标记source_parent_state=missing_campaign；不补假父、不猜V2关联。其余关联正常为observed。
- 成员必须通过源campaign/segment一致性检查，并关联本批真实segment history ID。广播recipient必须匹配plan.plan_id，message必须匹配同plan的recipient；缺失/矛盾保留archive并隔离，不造typed关联。
- Customer仅使用严格DM01 lineage/receipt/actual-target crosswalk。无映射为NULL，已有映射漂移不得降级为NULL。source campaign/segment ID只代表历史源键，不是当前V2外键。
- 原状态、历史计数（含负数）、空值与历史时间均保留；civil send_time不补时区。旧sent/queued不构成V2送达或重发授权。
- 正文仅沿既有历史消息连续手机号遮蔽口径输出；原正文、运行配置、身份、审批token等由sealed archive与payload digest保全，不恢复凭据或权限。
- Writer与journal在同一UoW事务；重放和对账比较完整typed目标摘要，包含生成ID。只读API/admin.read、有限分页、真实空/失败态，不回退Mock。
- 最小入口为7个GET：segments/list+detail、members/list、broadcast plans/list+detail、plan recipients、recipient messages。前端是现有Campaign页的独立历史模式，无启动/审核/重发按钮。
- 先完成114/115/116/117串行集中PR的候选与exact-main门禁，再发布本包；Schema118只在隔离验证完成后正式部署。不改V1、旧80/443/域名/回调或现有流量。
- V2网络隔离PG演练已完成五表25,480首次导入、全量重放和双次逐行对账，0隔离；seal0c5bebf5fc94154120c3bb3941cae4d3e299a7ab68d813a0562bc0041552f273。effects/event_log/river_job前后0。正式118、部署和普通用户人工验收尚未执行。
- 2026-08-28，前序Member Grid合并ff74127的exact-main33151096698已绿；本包以内容一致的父树对齐该主线，候选业务树未改。Go/race/vet、生成/契约/ownership及前端373/0已通过，本集中PR仍须精确候选Full绿后才能合并。

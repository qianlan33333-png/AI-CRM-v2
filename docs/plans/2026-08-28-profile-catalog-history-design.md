# V1 Profile Catalog：30条历史业务事实

从已验证main 2fa6f97隔离开发；集中包随115–119主线组合后才进入候选门禁，不提前正式120。只做用户要求的V1→V2领域历史落表，不恢复旧规则执行，不把历史读取当Excel当前模板能力完成。

## 冻结范围与最小验收

- Segment拥有template4/category10/option_mapping6；Contact拥有signup_tag_rules10。DDL120仅4张不可执行历史表。
- 源模板、类目、选项映射的numeric source IDs保持原值与命名空间；category的TemplateHistoryID必须解析到本包真实历史父；mapping的category/template组成复合FK，不跨模板绑定。
- 旧问卷、题目、选项、program、WeCom tag均保留source reference，不当成当前V2 ID，不额外猜配。旧模板version/sort/false/0/负值原样保留；created/updated转UTC微秒，actor仅摘要。
- signup规则只保留旧tag/name/status/active/time及source key/payload摘要，无当前tag目录写入、状态转移、Provider同步。
- app writer和migration journal必须共用caller Tx。重放核验源归档binding、receipt、目标全字段摘要与父关系；两次全量reconcile一致才可宣称本包迁移。
- 真实admin.read GET：模板列表/详情、按模板类目/选项映射、signup规则分页；最小只读页面复用现有后台。无保存/激活/执行动作，不退Mock。

## 收单边界

主代理独占Port/DDL/OpenAPI/路由/generated/CLI/中央reconcile/Matrix。子代理仅app/store/SQL查询或私有导入器、独立前端文件，不自行编号迁移或改共享契约。

顺序：本地与实际V2 network-none快照演练→集中PR CI→精确候选Full Nightly绿→合并→exact-main绿→V2正式迁移/独立测试部署。V1绝对只读，现网入口零切换；最终等待用户人工测试确认。

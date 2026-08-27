# V1 渠道：停用定义与历史归档

本包把可验证的旧渠道定义落入 Contact-owned `channels`，统一 `inactive`，
并将旧入渠/客服分配事实保存在独立只读历史表。沿用现有渠道列表/详情，
增加历史查询，不恢复二维码、获客链接、回调、客服权限或欢迎语执行。

| 源表 | 处理 |
| --- | --- |
| automation_channel | 编码、名称、类型、载体、原始时间；明确迁移 actor；`channels` + `channel_acquisition_legacy_archives` |
| automation_channel_assignee | `channel_historical_assignees` 保留历史人员引用和快照，不创建当前 staff 绑定或权限 |
| automation_channel_contact | `channel_historical_contacts` 保留历史入渠事实，客户只经冻结 DM01 证明关联；缺可验证来源时为 NULL，不改写客户当前 `channel_id` |
| automation_channel_entry_effect_log / entry_runtime | 归档，不创建运行任务 |
| automation_channel_qrcode_asset / scene_alias | 归档，不将旧 Provider 资产或路由当作 V2 可用资产 |
| channel_welcome_effect_dependency / effect_graph | 归档，不重放欢迎语依赖或执行 |

只允许从源带入 `channel_type`、`carrier_type` 和已校验名称/编码。
其余配置采用 V2 固定空默认：自动通过关闭，URL/scene/欢迎语为空，素材、标签、客服为空。
完整源 JSON 保留在既有加密归档；目标 legacy archive 只保存解密后源 JSON 的 SHA-256，
状态为 `legacy_unverified`。它不是 Provider 资产验证回执。

历史关系的渠道 FK 只使用同批已导入渠道的迁移回执，不复用 V1 数字 ID。
V1 客服分配时间没有时区，使用 `timestamp without time zone` 原样保留，API 明确为 civil timestamp。
定义/历史关系与迁移回执同事务写入；重放比较源摘要、目标 ID 和完整静态字段，
对账还核对目标停用、没有发布资产。重复运行不新增定义或操作记录。
源码缺失编码不补造成功定义，保留隔离原因和源记录。

```sh
aicrm-v1-domain-import --domain=channel --mode=import \
  --archive-run-id=<verified-archive-run> --migration-actor=<v2-admin-id> --dm01-run-id=<verified-dm01-run>
aicrm-v1-domain-import --domain=channel --mode=reconcile \
  --archive-run-id=<verified-archive-run>
```

独立版本 `v1-channel-a1`，九表对账与首批、静态、交易包分开，旧 seal 不重写。
源预检为 49 定义（1 条空编码）、316 入渠、14 客服分配、1,549 运行/资产历史行。预期结果不是部署证据；
只有实际 import/replay/reconcile 后记录数量。归档/隔离不计作业务功能上线完成。

V2 独立、无网络演练库实际验证：1,928 源行 = 378 import（48 渠道、316 入渠、14 客服分配）
+ 1,549 archive + 1 quarantine。重复 import 为 1,928 replay，两次 reconcile 均验证 1,928 行，
第二次 replay=true，digest `08a6a14b6e7a4d09d4c7c67959e79724f932d9d7921012585c3e3111466a375f`。
外部效果、渠道资产绑定、领域事件、River 任务、客户当前渠道归属计数均为零且不变。
演练已验证 110 迁移和真实 store 同事务回滚；这些结果不代表正式目标库或运行应用已部署本包。

本包不修改 V1，不修改 DNS、现网代理、企微/OAuth/小程序入口。
集中 PR 候选最终 SHA Full Nightly、合并后 exact-main Full Nightly 绿灯后才能在 V2 正式执行。
应用/数据回滚分开：数据恢复使用迁移前快照恢复到新隔离库，不删除目标行或不可变回执。

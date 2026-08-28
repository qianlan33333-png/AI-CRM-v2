# HXC 聊天任务历史隔离演练回执

范围：V2 已封存的 public/automation_laohuang_chat_job 全部 153 条、21 个源字段。仅写入 HXC 自有不可执行历史表；不创建当前任务、发送、重试、支付或 Provider 调用。私有文本、标识和原始 JSON 保留在领域表，不通过历史读取 API 暴露。

## 真实 PostgreSQL 演练

- 演练代码：e992a1daf7047c0cecfa9ffed5ef24e35028de7c。
- V2 隔离容器：aicrm-pre-automation-combined-dcf28fe，network=none；新数据库 aicrm_chatjob137。
- 使用现有 aicrm_cycle136 的 schema 副本，仅复制所需 153 条加密归记录、原始 ingress 和封存元数据；5 个既有演练库保留。
- provisional 137 迁移成功；真实 PG store round-trip/rollback 测试通过且未残留目标或回执。
- 首次导入 153，重放新增 0 / 复核 153；两次对账 selected/receipt/imported/verified 均 153，archive/quarantine 均 0。
- 一致摘要：e7942a961c7accb4fb5bcde1c4b52e71538e3ba7744224a0380c3c0e3d145103。
- 正式库 schema135、启动时间和归档计数未变；演练 runtime/event/queue 仍为 0；容器结束后 exited，network=none。

证据目录：/data/aicrm/chatjob137-rehearsal/evidence-e992a1daf7047c0cecfa9ffed5ef24e35028de7c。

- reconcile.json SHA256：713b81597b57cf73c3c3bf4456b22037148ae88e618ed48c2600c0b3501a35d7。
- reconcile-replay.json SHA256：d5ba66f51b212e88dae89ac9ba64174e081488d749f6da5d8ca2f26ccbc410f9。

本回执不表示正式137已迁移、部署或全P4验收完成。最终编号/契约须在前序已验证 main 上串行集成，并通过 PR CI、最终候选 Full 和 exact-main Full。V1 零修改，旧生产流量未切换；真实登录、Sidebar 和 Provider 效果单独验收。

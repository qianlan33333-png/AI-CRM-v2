# 优惠券历史最小落表

执行已授权的V1只读、V2正式落表与独立人工验收计划。源为冻结归档的13定义、
13商品绑定、26领取、16核销；不得恢复公开链接、发券、支付或退款执行。

## 选择

复用现有coupons/targets，定义固定archived；新增一个Coupon-owned历史标记表及
领取/核销两张只读历史表。标记只保存source_coupon_id与original_status，来源HMAC
和批次仍由既有迁移journal管理。原tenant、unionid、幂等密钥、public_slug及文本actor
不复制为V2业务身份或权限；unionid仅在迁移命令内部用冻结DM01解析可信客户。

直接复用运行态claims会丢失状态、金额、期限和核销事实，不采用；仅留archive不能
供用户在业务页面逐项浏览，也不采用。历史定义通过只读页展示，当前mutation读取
owner历史标记后拒绝修改/复制历史券，不新增通用风控或权限体系。

定义writer同一caller事务先插archived且issued_count=0的header、全量targets，再恢复
源issued_count/first_claim_at，最后写历史标记及journal；遵守现有已领取券触发器。
绑定只接受已封账Product映射；客户沿DM01、订单沿Finance历史receipt与实表校验。
不能验证的引用保留NULL或明确隔离，不能把旧数字ID直接当V2外键。

领取/核销保留原status、金额、日期及可空可信引用；不进入coupon_claims、原生退款、
operation receipts、event_log、River或Provider。新增GET只读分页，不扩当前领券API。
迁移112由主线串行落地，须等待110/111及本包候选、exact-main门禁。

## 验证

逐表源行守恒、真实PG同事务回滚、68行实源预检/导入/重放/对账、目标摘要漂移拒绝、
错误映射关闭、历史券不可变、只读接口鉴权与空态/失败态。V1零写入、现网零切换。

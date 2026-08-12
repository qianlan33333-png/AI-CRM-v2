# P3-C06A2 性能验收器机制证据

- evidence_class：`LOCAL_MECHANISM_ONLY`
- base_sha：`47fbaebce120ef9f4f128f94588d396abb128da3`
- branch：`slice/p3-c06a2-performance-runner`
- focused race/vet：`PASS`
- 4096 场景与收据严格负例：`PASS`
- source-policy 精确双路径与复制路径负例：`PASS`
- repo-contract checker 与永久负例全集：`PASS`
- serial full Go gates with real PG16 on 55432：`PASS`
- Web CI Node24/npm11 186 tests/build/audit：`PASS`
- gitleaks：`PASS`
- 独立复核最终结论：`PASS`，candidate `a9ee219c739f8ddd4b01a60bb216c65a6e814489`，
  report SHA256 `0ef6d1ab895db4410dd421f9136b852279b57f8c5e6d7621741f3eb22ac528eb`
- PR：[#138](https://github.com/qianlan33333-png/AI-CRM-v2/pull/138)
- merge SHA：`ea6e9771dfa02db06ec793a1aebdad89826eabcc`
- main CI：application `31572854039`、repo-contract `31572853990`、secret scan
  `31572853988`，均绑定 merge SHA 且 PASS。
- 授权测试服务器运行：`EXECUTED — P3-C06B first baseline hard gate failed`
- 20 万客户任意组合 P95 `<200ms`：`FAIL`（80 个场景 P95 `>=200ms`）
- 真实 EXPLAIN：`EXECUTED`（8,192 份；无目标 Seq Scan；完整收据 SHA256
  `8c15d9a5485241e0f51a0f4f94572d0e818d843b980ffaa6b35d1b693e0e8efb`）
- 生产数据库/真实企微/live migration：`NOT EXECUTED`

机制和首轮失败只证明 runner 会 fail-closed；不得把本文件作为 S 档性能通过证据。原始
EXPLAIN JSON 会随每个场景保存，并由离线验证器重新解析核对摘要。C06B 的收据
必须绑定 A2 merge SHA、main CI、测试机环境、隔离库事实和实际 4096 场景。

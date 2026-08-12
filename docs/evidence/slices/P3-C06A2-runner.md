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
- 独立复核初次结论：`FAIL — main CI URL 与完整 EXPLAIN 存证缺口已修，待复核`
- 授权测试服务器运行：`NOT EXECUTED — 留给 P3-C06B exact-main binary`
- 20 万客户 P95 `<200ms`：`NOT EXECUTED`
- 真实 EXPLAIN：`NOT EXECUTED`
- 生产数据库/真实企微/live migration：`NOT EXECUTED`

机制只证明 runner 会 fail-closed；不得把本文件作为 S 档性能通过证据。原始
EXPLAIN JSON 会随每个场景保存，并由离线验证器重新解析核对摘要。C06B 的收据
必须绑定 A2 merge SHA、main CI、测试机环境、隔离库事实和实际 4096 场景。

# P3-C02B Terra service 测试收据

- 执行任务：`/root/p3_c02b_service_tests`
- 原冻结 SHA：`9bc858d4394f68c8045a0531b46eb7ec7a28d59c`
- replay parent：`61f6e685f54bb97992c7fa9adf3efb9c1a523ec3`
- Terra head：`3360058c0e14fe767a1819981d9f2ee8984b26c9`
- manifest SHA-256：`e203df2fac964702a70eead8358327d40ea1baaa60e2cc682436485c461a1518`
- binary diff SHA-256：`94cd6f632d75174ecd1a8340d3c283deb4ad548e2b64d574e0825846e3c8aa99`
- correction：`slice=0 / infra=0 / scope=0 / verification=0`

Sol 在集成前独立复算 parent、单文件 manifest 与 binary diff，结果逐项匹配；
集成后运行 contact app focused race test。Terra 未 push、未创建 PR、未 merge。
Sol 后续补充了渠道中立 `extra` 的回归语料；以上 hash 只绑定原始委派载荷。

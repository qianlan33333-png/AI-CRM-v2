# P3-C02C Terra store scope 测试收据

- task：`/root/p3_c02c_store_scope_tests`
- executor：Terra Max
- frozen base / parent：`abc3896bff3a9dd6def7b22ad586841bb47a718e`
- delegated head：`ad7ce39e88429479223958edc6dadb5dba309fd8`
- manifest SHA-256：`cdf3b82eadc99449175445353e54498a335706ac68f5f63103e681bfdcce492e`
- binary diff SHA-256：`a8459ccd7dfa7eacb855004f26dcf658c9c26073de30879f6f2b69f31df75abc`
- correction：`slice=0 / infra=0 / scope=0 / verification=0`

规范 manifest：

```text
100644 36316 6736dc9263dc0db994037e537be007e4684fc993b8a151ab5e3c5dd2aeaf119c internal/contact/store/customer_mutation_repository_test.go
```

Sol 在集成前独立复算 parent、单文件 manifest 与 binary diff，结果逐项匹配；
集成后运行 contact store focused race test。Terra 未改生产代码，未 push、未建 PR、
未 merge。

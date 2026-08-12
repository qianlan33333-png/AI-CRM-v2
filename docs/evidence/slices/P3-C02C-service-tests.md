# P3-C02C Terra service scope 测试收据

- task：`/root/p3_c02c_service_scope_tests`
- executor：Terra Max
- frozen base / parent：`abc3896bff3a9dd6def7b22ad586841bb47a718e`
- delegated head：`3e2a86d36a4a01ca1a89cc7fb0e3eae874df0560`
- manifest SHA-256：`28b61504fd99db1e3e0633f03d66b2c82757b8ef9b87aac33c0e103698e08b15`
- binary diff SHA-256：`ff729c4e1cebe710c9936dced17e67cd61bcd8cf776f792a36a88451b09fcfa6`
- correction：`slice=0 / infra=0 / scope=0 / verification=0`

规范 manifest：

```text
100644 42108 e2fb1370e6f348a155e667c4f30ce27ae5b5d98d32622e9a92cc265c5e8a59b1 internal/contact/app/customer_mutation_service_test.go
```

Sol 在集成前独立复算 parent、单文件 manifest 与 binary diff，结果逐项匹配；
集成后运行 contact app focused race test。Terra 未改生产代码，未 push、未建 PR、
未 merge。

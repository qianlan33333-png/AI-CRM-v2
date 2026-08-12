# P3-C02D Terra service 测试收据

- task：`/root/p3_c02d_service_tests`
- frozen base：`527d833fa4a93f04e699bd583e293f335f0691bd`
- delegated head：`ec61090e7318fb094e84d22383f85e016ce8973f`
- manifest SHA256：`7f70eff7683f8620790c4487f886dd27a5c413cfc9a41b609f9f2a52e9cd9908`
- binary diff SHA256：`715eecf3441905d97d22c09d010d667146be4624de693f47dd43e48b7cc56071`
- correction：`slice=0 / infra=0 / scope=0 / verification=0`
- finding：重复 cursor JSON key 在冻结实现中被接受；Sol 已以严格唯一字段解码修复，
  回放及集成后 focused race 均通过。
- Terra 未 push、PR 或 merge。

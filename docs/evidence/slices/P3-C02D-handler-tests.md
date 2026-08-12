# P3-C02D Terra handler 测试收据

- task：`/root/p3_c02d_handler_tests`
- frozen base：`527d833fa4a93f04e699bd583e293f335f0691bd`
- delegated head：`9d23c9863a0e7fd15f242e151d3ea78df343da7a`
- manifest SHA256：`b2b14d790ad9dc0d43a921f9140bf4289e3a9b46e921c9e46744278c6a8af223`
- binary diff SHA256：`75ce89c76b99f736fd285e53db4266dc86549159cf644eb40981ce668e275b8d`
- correction：`slice=0 / infra=0 / scope=0 / verification=0`
- finding：HTTP 转换未校验 application item 与请求 customer ID 一致；Sol 已传入
  expected customer 并 fail-closed，永久负例和 integrated focused race 均通过。
- Terra 未 push、PR 或 merge。

# P3-C02A Terra service 测试语料收据

- task：`/root/p3_c02a_service_tests`
- executor：Terra Max
- original frozen base：`147d5b3f9ad2a9f55d2dd87a9b4f472f9593b7ea`
- replay base / parent：`b83765a77d067dfe380f2ef2732812e09c29ab8e`
- delegated head：`724cb62571e9a1293b9036e0bfd48d09a040a5f9`
- manifest SHA-256：`06675e424e9e75daac428f378ff24b229e7ff4789cdc52d7ef57d90902f926a3`
- replay base..head binary diff SHA-256：
  `88942a536f02ff027bb30f38374e27ea748a515a050ec88f74ac02eefe34ff0e`
- Sol 独立复算 parent、manifest 与 binary diff：PASS。
- focused race：PASS；锁定 update no-op 零事件/零 key、事务写序、失败传播与重试 key。
- Terra correction：`slice=0 / infra=0 / scope=0 / verification=1`。
- Terra 未 push、未建 PR、未 merge；Sol 已复核并集成。

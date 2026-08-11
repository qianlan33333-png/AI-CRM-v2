# P3-C03 Terra 分区维护 worker 测试收据

- task：`/root/p3_c03_partition_worker_tests`
- executor：`gpt-5.6-terra / max`
- frozen base：`b9605b4e3336f578debc2e1bd79593283166d733`
- delegated head：`2f6b4e1350c050bb23bf6b2f695b3ee50b26b5ba`
- parent：精确等于 frozen base
- correction：`slice=0 / infra=0 / scope=0 / verification=1`
- worktree：clean；未 push、未 PR、未 merge

## 规范 manifest

```text
100644 3966 3227e6707531282ba1520000bdd565d41a4b479f6d4775af12155df1494b9718 internal/contact/worker/event_partitions_test.go
```

- manifest SHA-256：`ac7759ec5a3e41a4818ae46e04642bef399c22febd71f53ca0c6cce65efcea34`
- binary diff SHA-256：`df75097a5c541b1a262f801455684ab90e0a7b019b0a1d879f88754689002f2a`

## Sol 复核

- parent、唯一路径、manifest 和 binary diff 已独立复算一致。
- 测试只覆盖 Sol 先冻结的 Kind、依赖边界、UTC anchor、horizon=3、错误传递
  和 timeout，未触碰生产代码、migration、公共 port、中央合同或 GitHub/main。
- Sol 集成生产 worker 后 focused race 真实通过；本文只记原始 Terra payload，
  不冒充后续 Sol 产出。

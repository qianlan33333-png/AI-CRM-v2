# P3-C04 Terra 客户列表 UI 收据

- task：`/root/p3_c04_customer_list_ui`
- executor：`gpt-5.6-terra / max`
- frozen base：`efe1b961603be8470a595a064b3367c8ef3a0346`
- delegated head：`417e6d24e7320526ef9b89b645c682a7b421407c`
- parent：精确等于 frozen base
- correction：`slice=2 / infra=1 / scope=1 / verification=1`
- worktree：clean；未 push、未 PR、未 merge

## 规范 manifest

```text
100644 3904 e2e9522f30b1cd44606667f4372bb5fb76b143111a36bac36bf625ed3e6a8b3e web/src/customers-list.css
100644 5850 87c3b414351ff52237fbee09403a6d311157fcd36a9d90944d4f25723f3e4650 web/src/customers-ui.test.tsx
100644 15288 a9b6d658a5c15b32fae61aa51f0ec9d7f6d72a2fb2330bc7fc7e7481b8ef25e0 web/src/customers-ui.tsx
100644 8369 2cde559601f73f7f09ea21fb38678483384d950f066cbacb2762bfca0f8027b8 web/src/customers.test.ts
100644 13028 fbbeca519202ee0d556c6c512fd50843bd4f1e8ade993cf8a2305852f659e678 web/src/customers.ts
```

- manifest SHA-256：`9aa5f4d17f27ee1c2ae4692fb323bfb05dd64de7946e93c13ddabe48375ef83b`
- binary diff SHA-256：`b75d76b5dc0faf8d66f2569e5bebed19224ab9e2629046d924c5e51617088a89`

## Sol 复核

- parent、唯五路径、manifest 与 binary diff 独立复算一致。
- 不包含 OpenAPI、生成物、公共 port、根依赖、GitHub 或 main 操作。
- Sol 在最新 main 上集成后，独立补充 shell 路由、avatar 安全边界、门禁与
  证据；该部分不冒充 Terra 原始 payload。

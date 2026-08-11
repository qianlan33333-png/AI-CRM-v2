# P3-C01B Terra sqlc 交付收据

- task：`/root/p3_c01b_sqlc_queries`
- executor：`gpt-5.6-terra / max`
- frozen base：`4f9a5a51e9d49f4bf3abb06ddeff048306b79e69`
- delegated head：`22a98b177d390241fcc7e3f85b117fbaa83ee401`
- parent：精确等于 frozen base
- correction：`slice=1 / infra=0 / scope=0 / verification=0`
- worktree：clean；未 push、未 PR、未 merge

## 规范 manifest

```text
100644 6505 6050f071208c294d3c9aadb61187349e4c554efda0c7230e6877ce10c55ef0de internal/contact/store/customer_query_repository.go
100644 12914 a8bcc0dcfd0931601773c96ab3bf6225dcbab59c7363c8f8f5be0516c652d59b internal/contact/store/customer_query_repository_test.go
100644 5832 4c77bb954881911f0ec29c4fbe4817b9401bbbc5360dc56279cd3f1490c0a64a internal/contact/store/generated/customers.sql.go
100644 1017 9459ba27d0397425970580f71f26f1871214fcd1cbb0b1eb48bb1143a97ec956 internal/contact/store/generated/models.go
100644 545 4010d8205aa7a3c8df4001724f91960271f3dcaa8c21f85b18cbe920b1c4f61f internal/contact/store/generated/querier.go
100644 3339 5491c2597157e433f1cd2195683a466a8a90f12031d0d6808e53479cb9e2b5dc internal/contact/store/queries/customers.sql
```

- manifest SHA-256：`040329fc43d9e21d3156db064dce955a756997808c80e6513d4ba504cb99b3aa`
- `git diff --binary --no-ext-diff BASE..HEAD` SHA-256：
  `04624a5a2b48da004d515e7235feeed108c285f5d54f792966e271933dcf62a1`

## Sol 复核

- 白名单、parent、manifest 与 binary diff 独立复算一致。
- 首版 `COUNT(*)` 未通过 Sol 语义复核；最终版完全移除 COUNT 并补静态负例。
- sqlc v1.28.0 连续生成、focused race 与 query-plan gate（checked=2）通过。
- Sol 另加隔离 PostgreSQL 16 黑盒验收，不使用 Terra 的 fake transaction 冒充真实 PG。

# P2-11 Terra Max 测试语料收据

- task：`/root/p2_11_gateway_test_corpus`
- executor：`gpt-5.6-terra` / `max`
- frozen base：`45d6dadb077db71964ced198d350b236747908c7`
- delegated head：`21661dbc0400908bbb603ebafb57ec445d586ead`
- allowed path：`internal/platform/http/gateway_test.go`
- correction_count：`0`
- Git 权限：仅本地 commit；未 push、未创建 PR、未 merge

## Manifest

```text
100644 30214 2f6e896761be30b31248f41bc6535eaebdbd3d7afe0b433fcc80aafbb1930060 internal/platform/http/gateway_test.go
```

- manifest SHA-256：`2cab3482eb92da67947a16f3b723a664a0250e374a48b4fe21a301849be71cba`
- `git diff BASE..HEAD --binary` SHA-256：
  `bc284eb025362d30d36d12fff33da514b4233bf924350ccd057900ea84587496`

## Sol 独立复核与集成

- `HEAD^` 精确等于 frozen base，`BASE..HEAD` 仅修改白名单文件；Sol 从
  Git object 独立重算 mode、bytes、文件 SHA-256、manifest 与 binary
  diff SHA-256，结果如上。
- 冻结 base 上的完整包只有新增 401/429 短路日志断言预期 RED：
  两者均为 `decode access log "": EOF`，证明测试确实捕捉本片修正前的缺口。
- 集成时删除了未冻结的“并发上限 64 / 超时上限 5 分钟”语料，保留
  负数 fail-closed 与卡片允许更短专项超时的合同；该次计为
  `verification_induced=1`，不改动生产规则。
- Sol 集成后执行
  `GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go test -race -count=1 ./internal/platform/http ./cmd/aicrm ./acceptance/p2s08 ./acceptance/p2s11`
  全部通过。

# P2-08 Terra Max HTTP 网关测试语料收据

## 原始委派

- executor：`gpt-5.6-terra`，reasoning `max`
- base：`a6107db82e97e685a780065cce52db7d1320fa86`
- delegated head：`291add7ede9ed68dd2b5227c7ade048021f6a3a2`
- allowed path：`internal/platform/http/gateway_test.go`
- correction_count：`0`
- Terra focused test：`go test -race -count=1 ./internal/platform/http`，PASS

```text
100644 16629 e452c5c0bc5834d187ea1f6980855cb57798322adeeb37776cf0f2550874b96c internal/platform/http/gateway_test.go
```

- file manifest SHA-256：
  `d27df4b0e00503e4d4e85c1a2f485ab60b886a413b18b4f3041107b9efabec83`
- `base..delegated head` binary diff SHA-256：
  `64c0e31b7f35da1d893754f39ff6bc0403d080fd484459ef87d5890413719d99`

## Sol 独立复核

- 祖先关系、唯一路径、mode、bytes、文件 SHA、manifest SHA 与 binary diff SHA
  已逐项独立重算，全部与 Terra 原始收据一致。
- 审读后发现测试 helper 用 `json.Decoder.More` 检查顶层第二条日志并不成立；
  Sol 改为第二次 `Decode` 必须返回 `io.EOF`，归为 `verification_induced=1`。
- 修正后独立执行 `go test -race -count=1 ./internal/platform/http
  ./acceptance/p2s08`，PASS。最终文件 hash 由 repo-contract 冻结，不回写冒充
  Terra 原始产物。

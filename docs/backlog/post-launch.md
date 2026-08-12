# Post-launch backlog

本文件是上线前已知但明确推迟事项的唯一出口。每条必须保留来源、旧行为、推迟原因与建议时机；安全、数据损坏/不可逆风险、已决 ADR、全部架构铁律与 CI 门禁均不得进入本表逃避处理。

| 来源片 | 旧行为 | 为何暂不处理 | 建议处理时机 |
| --- | --- | --- | --- |
| SEC-01 / GO-2026-5777 | 根依赖保留 `github.com/go-chi/chi/v5@v5.2.3`；其 `middleware.RealIP` 存在未校验 `X-Forwarded-For` 的 IP spoofing 风险。 | `govulncheck -show verbose ./...` 仅列为 Module Result，当前根包没有导入或调用受影响符号；按用户裁决不在 SEC-01 扩大升级范围。 | 首次引入 RealIP、代理来源 IP 信任或相关中间件前，升级到至少 `v5.3.0` 并补可信代理边界测试。 |
| SEC-01 / GO-2026-5775 | 根依赖保留 `github.com/go-chi/chi/v5@v5.2.3`；相关 middleware 对 `X-Forwarded-For` 的处理可导致 IP spoofing。 | 仅为 Module Result，当前代码不可达；SEC-01 只修复已知会被 P2-00 变为可达的漏洞。 | 首次启用来源 IP 鉴权、限流或审计前，升级到至少 `v5.3.0` 并重新运行全量漏洞扫描。 |
| SEC-01 / GO-2026-5774 | 根依赖保留 `github.com/go-chi/chi/v5@v5.2.3`；`middleware.RealIP` 存在 IP spoofing 风险。 | 仅为 Module Result，当前没有受影响调用链；依照用户指令留痕而不顺带升级。 | P2 HTTP 中间件片冻结可信代理策略时，升级到至少 `v5.3.0` 并验证伪造转发头负例。 |
| SEC-01 / GO-2026-4316 | 根依赖保留 `github.com/go-chi/chi/v5@v5.2.3`；`RedirectSlashes` middleware 存在 open redirect 风险。 | 仅为 Module Result，当前根包未调用受影响 middleware。 | 首次启用 `RedirectSlashes` 前升级到至少 `v5.2.4`；若不需要该能力，保持不启用并在 P2 HTTP router 片复核。 |
| P3-C07A / slice 治理文档漂移 | `docs/spec/AI-CRM-v2-执行方案-v2-至P3.md` 附录 A 仍写“slice 达到 2 / 两类合计 2 停报”，与当前 `AGENTS.md` 的“仅 `slice_induced >= 3` 停报”冲突。 | 按仓库优先级与用户最新裁决，本次只执行 `AGENTS.md`，不在 Contact 行为片顺带改冻结规格。 | 下一次独立治理/spec refreeze 片同步附录 A，并保留本次冲突与裁决记录。 |

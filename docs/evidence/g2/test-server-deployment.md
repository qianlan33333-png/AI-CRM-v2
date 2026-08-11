# G2 测试服务器部署证据

## 结论

2026-08-11，已在用户授权的新 2C4G 低用户量测试目标完成 AI-CRM v2
后端部署。运行版本精确绑定：

`05247dd16da571af9fefb081c46eb9facb2ddc38`

该结论表示测试服务器后端、数据库、队列和服务器本机 HTTPS Web 边缘层
运行通过，不表示公网 HTTPS、浏览器登录、Stages CRUD、真实企微链路或
G2 人工门已通过。

## 源码与 CI

- PR：[#118](https://github.com/qianlan33333-png/AI-CRM-v2/pull/118)
- PR head：`b210ccee40a9c7d7be0136453c696cf43eb802c1`
- squash merge：`05247dd16da571af9fefb081c46eb9facb2ddc38`
- exact-main application/Web：run `31501801845`，PASS
- exact-main repo-contract：run `31501801808`，PASS，负例总耗时 18m14s
- exact-main secret scan：run `31501801884`，PASS

## 目标与发布物

- 目标规格：2 vCPU、约 4 GiB RAM，active swap 大于 4 GiB。
- Docker `29.1.3`，Compose `2.40.3`。
- PostgreSQL：`postgres:16.14-bookworm`，服务端版本断言 `160014`。
- 首次传输归档 SHA-256：
  `e489afb0e2227fb4933dc3a55c5fb37093ac40a4d850f6b149be7a455084b735`。
- 真实文件 `SHA256SUMS` 全部通过，`SOURCE_SHA` 与 main SHA 精确一致。
- 归档发现的 23 个未跟踪 AppleDouble 文件已移出 release 目录隔离；隔离前
  Goose 尚未执行任何 migration。清洁打包器对同一真实 release 重打包结果
  SHA-256 为
  `8b04642069a97f0c78023ee4b4a0f2bf3290dcde477ef4d04fdf0fce6eec1f1b`，
  列表中无 `._*`。

仓库未记录服务器地址、账号、密码、数据库随机密码或其他可复用凭据。

## 数据库与迁移

执行顺序固定为：PostgreSQL healthy → Goose up → River up → application。

- Goose 成功应用 `00001` 至 `00004`，当前版本 `4`。
- River 官方 migration up 成功，`river_migration` 共 `6` 条记录。
- 可见业务/基础表包括 `event_log`、`settings`、`settings_audit`、
  `admin_users`、`admin_sessions`、`stages` 与 River 表。
- PostgreSQL 仅位于 Compose 网络，宿主机没有 5432 listener 或 port
  mapping。
- 未连接生产数据库，未执行 legacy 数据读取、数据导入或 live migration。

## 运行时

- image revision label 精确等于部署 SHA。
- Linux `amd64`，固定用户 `65532:65532`。
- root filesystem read-only，`cap_drop=ALL`，
  `no-new-privileges=true`。
- API 仅绑定 `127.0.0.1:8080`；外部探测公网 8080 不可达。
- `/healthz` 返回 `200` 与 `{"status":"ok"}`。
- `/api/v1/auth/session` 返回 `401`，未创建测试后门或伪造已登录状态。
- 六个队列注册为 `critical/event/outbound/sync/heavy/ai`；S 档并发为
  `2/1/1/1/1/1`，worker PG 池为 `9`。
- 应用容器重启后 `/healthz` 再次通过，六队列仍为 6 条。
- 启动与重启日志未发现 panic、fatal、结构化 error 或数据库密码。
- `/opt/aicrm/current` 仅在全部健康证据通过后绑定精确 SHA release。

## 未执行项

- `scripts/staging_deploy.sh --apply`：`NOT_EXECUTED`。本次需先 migration
  后 app，故按安全顺序显式执行 Compose；不能把 render/apply 脚本输出当作
  真实部署证据。
- 公网 HTTPS 443：`PENDING_EXTERNAL_GATE`。服务端 Caddy 已监听 TCP/UDP
  443、UFW inactive、nftables INPUT accept；公网 HTTP 80 可达并返回 308，
  但外部 TLS 连接在云侧安全组处失败。
- 浏览器登录与 Stages CRUD：`PENDING_EXTERNAL_GATE`。
- 真实企微 OAuth、回调、加好友、群发和手机收信：
  `PENDING_EXTERNAL_GATE`。
- 生产数据库只读 preflight、live migration 和生产部署：
  `PENDING_EXTERNAL_GATE`。
- G2 人工签字：`PENDING_EXTERNAL_GATE`。

## HTTPS Web 边缘层追加证据

- 合同 PR #121，修正 PR #122；最终代码与 Web 发布物绑定 main
  `68b3b7e3dd35b95e5e785ed89ba23bb9029f89b5`。
- exact-main CI：application/Web `31518175824`、repo-contract
  `31518175761`、secret scan `31518175802`，全部 PASS。
- Web 归档 SHA-256：
  `a19c533e7dfa19305bf1489193b09641c4a2e472323f18cfc8a045c88448fcc3`；
  发布根为 root-owned `/var/www/aicrm`，未放宽含运行配置的 `/opt/aicrm`。
- Caddy `v2.11.4` 官方 Linux amd64 资产 SHA-256：
  `c41708ffb4af9bc6d19f7d22a7a034804352a21ecc62e1d3dfe3d58e30b38a3e`。
- 服务器本机真实证书校验成功：首页和 `/healthz` 均为 200；证书 SAN
  `aa.youcangogogo.com`，Let's Encrypt `YE1` 签发，有效期至
  2026-11-09 16:22:52 UTC。

## 清理与回滚

- 服务器端两个临时传输归档已按精确路径永久删除：release archive
  `20,636,775` bytes、PostgreSQL image archive `442,914,304` bytes；不可恢复。
- 已加载镜像、PostgreSQL volume、精确 SHA release 与本机清洁归档保留。
- 回滚时先停止应用；测试数据无保留要求时可删除 staging volume 并从空卷
  重建。此策略不适用于生产数据。

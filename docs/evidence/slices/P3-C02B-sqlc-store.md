# P3-C02B Terra sqlc/store 收据

- 执行任务：`/root/p3_c02b_sqlc_store`
- 原冻结 SHA：`9bc858d4394f68c8045a0531b46eb7ec7a28d59c`
- replay parent：`61f6e685f54bb97992c7fa9adf3efb9c1a523ec3`
- Terra head：`30ef10541c21fcb70ce219fb6653f4fc51f16d05`
- manifest SHA-256：`b76292e5dc33284d429d2544ba42708a66a524d3ccd4c0d3375636175018e92e`
- binary diff SHA-256：`9df552135fcb5b15da818af80abf6afe5f2c366b82f2a9ca63cb1f08d484546e`
- correction：`slice=0 / infra=0 / scope=0 / verification=1`

Sol 在集成前独立复算 parent、五文件 manifest 与 binary diff，结果逐项匹配；
随后运行 store race test、sqlc 连续两次生成无 diff，并把生成物纳入冻结 manifest。
Terra 未 push、未创建 PR、未 merge。

独立复核随后发现两条 SELECT 不满足同一 statement snapshot；Sol 已把最终实现改为
单条 SQL。以上 hash 只绑定原始委派载荷，不冒充最终树收据。

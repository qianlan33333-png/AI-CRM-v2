# P1-C01 legacy API 对照候选

- 提取来源：保留分支 `slice/p1-s10-api-mapping@1b2e8bc6a948d7652b29806dbbcd2cd2caffe52d`；重落 base `4ba0fc6c0a69b03037707cf9cb0fb4f0267afa75`，legacy `6cb989c071255437d75953dabb943318a74eb8f4`。
- [`api-mapping.jsonl`](api-mapping.jsonl) 有 781 条唯一 mapping ID 和 781 条唯一 route key，分区为 `S02=156 / S03=184 / S04=441`，与 P1-S01 manifest 路径、route name 及排序 methods 逐键相等。
- 重落时将 49 条候选中缺失的 `client_purpose/service_audience/service_capability` 机械补齐为当前 P1-S01 权威 manifest 完整对象；其余候选事实与状态不变。
- 64 个 handler 文件、51 个 HEAD/OPTIONS source-only method；input/output 各有 121 条 `PARTIAL_IMPORTED_SCHEMA`，只表示允许范围内的静态局部证据，不等于运行时 payload。
- 781 条均为 `UNREVIEWED / PENDING_HUMAN_DESIGN / PENDING_HUMAN_SIGNOFF`，`decision_evidence=[]`；本片不修改 OpenAPI，不新增中央 CI 接线，不冒充 G1 已签字。
- 内容 SHA-256：`06592b04603689472e11da095a1865104bcf95ce59ac887a3559db0ad3c76dfe`。

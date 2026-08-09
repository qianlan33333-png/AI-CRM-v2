# T1.1 legacy API 对照候选

- v2 base `b3613f635692c932021036f8f81babf24fca8222`、legacy `6cb989c071255437d75953dabb943318a74eb8f4`；[`api-mapping.jsonl`](api-mapping.jsonl) 对账 781 条 route（`156+184+441`）、64 handler、51 个 HEAD/OPTIONS 辅助 pair；输入/输出各 121 条 `PARTIAL_IMPORTED_SCHEMA`，均不等于运行时 payload；所有处置为 `UNREVIEWED/PENDING_HUMAN_SIGNOFF`，不得修改 OpenAPI，completion 此前返回 exit 2。

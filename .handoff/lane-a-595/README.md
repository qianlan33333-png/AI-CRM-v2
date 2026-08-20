# Lane A exact-main source chunks

This branch is a temporary read-only handoff for ChatGPT Pro. It is not a product change and must not be merged.

- Repository: `qianlan33333-png/AI-CRM-v2`
- Exact base commit: `595f6ba8092879b4166849f043e2315a4b7b9ca3`
- Exact root tree: `7c6a21365961f02f15ad1b71ffa30cabcab3b5e9`
- Chunk size: 48 KiB except each final chunk

Reassemble each directory in lexicographic `part-000`, `part-001`, ... order without separators:

| Chunk directory | Destination | Bytes | Expected Git blob |
| --- | --- | ---: | --- |
| `openapi/` | `api/openapi.yaml` | 508033 | `30743d1b79aeccf03d3db3508f7361ebe0cbd1bf` |
| `server-gen/` | `internal/api/candidate/generated/server.gen.go` | 602536 | `bce9b1490c714c9edbe21003bcfb328fb8997862` |
| `health-ts/` | `web/src/api/generated/health.ts` | 695111 | `58873720311e2c465a7c7204636d0a7425acb07a` |

`CHUNK_SHA256SUMS.txt` gives every individual chunk SHA-256. After concatenation, run `git hash-object <file>` and require the exact blob above before using the file as a `595f6ba` preimage.

These chunks contain exact bytes from `595f6ba`; do not reinterpret, normalize line endings, or copy rendered Markdown. Fetch each file through the GitHub connector/API as raw file content and concatenate the decoded bytes.

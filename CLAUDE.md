# CLAUDE.md

开始任何任务前必须完整阅读根目录 `AGENTS.md`、相关 ADR、架构总纲和
当前 Slice 卡。`AGENTS.md` 的权限、架构铁律、OneID、生成器、证据和
Slice 限制全部适用，不在本文件重复定义。

外部实现代理只能修改任务卡白名单中的文件。若冻结契约、DDL、OpenAPI、
公共 port 或黑盒测试存在问题，应停止编码并提交问题报告，不能自行改变
架构。未实际运行的测试必须标记 `NOT EXECUTED`；不得把 mock 或 synthetic
结果描述为真实外部链路。

# L2 测试工厂执行方案索引

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。

## 文件清单

| 文件                                | 作用                                              |
| ----------------------------------- | ------------------------------------------------- |
| 01_xlib-standard_execution_plan.md  | 标准源、模板、Registry、Release Level             |
| 02_testkitx_execution_plan.md       | Contract Runner、requirex、能力族测试运行时       |
| 03_xlibgate_execution_plan.md       | 机器裁判 CLI，替代长期 l2gate                     |
| 04_redisx_execution_plan.md         | KV / TTL / Lock / Stream / PubSub                 |
| 05_postgresx_execution_plan.md      | SQL / Tx / Pool / Migration                       |
| 06_natsx_execution_plan.md          | PubSub / RequestReply / JetStream                 |
| 07_kafkax_execution_plan.md         | EventLog / Producer / Consumer / Offset / DLQ     |
| 08_ossx_execution_plan.md           | ObjectStore / Aliyun OSS v1.0.1                   |
| 09_clickhousex_execution_plan.md    | ColumnStore / Batch / Analytics / TTL / Partition |
| 10_taosx_execution_plan.md          | TimeSeries / Stable / ChildTable / GapCheck       |
| 11_xgo-market-data_adoption_plan.md | L4 market-data 下游采纳                           |
| 12_xgo-macro-data_adoption_plan.md  | L4 macro-data 下游采纳                            |
| 13_engines_adoption_plan.md         | L5 market/macro/regime engine 采纳                |
| 14_xgo_runtime_system_gate_plan.md  | L6 x.go Runtime Gate 与 System Gate               |

## 推荐执行顺序

```text
1. xlib-standard 标准源
2. testkitx common + kv
3. xlibgate/l2gate MVP
4. redisx L2-T2 → L2-T3 → L2-T4
5. postgresx L2-T2 → L2-T3 → L2-T4
6. natsx / kafkax
7. ossx / clickhousex / taosx
8. xgo-market-data / xgo-macro-data
9. engines
10. x.go runtime + system readiness
```

## 硬规则

```text
没有 capability manifest，不知道测什么。
没有 contract pack registry，不知道怎么测。
没有 testkitx runner，测试不可复用。
没有 xlibgate，标准不可强制。
没有 Evidence，不允许 DONE。
```

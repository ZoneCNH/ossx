# xgo-macro-data 执行方案：L4 Macro Data 下游采纳门禁

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`xgo-macro-data` 是 L4 宏观数据服务层，负责宏观数据采集、归档、宽表、事件发布和 information set 治理。

## 2. 允许使用的 L2

```text
postgresx    metadata / release calendar / state
ossx         raw response archive
clickhousex  factor wide table
kafkax       macro update event
redisx       optional current snapshot
```

## 3. 关键原则

```text
必须遵守 information set。
必须记录 release_time / as_of_time / collected_at。
原始响应必须归档。
修订数据必须记录 revision / vintage 或修订风险字段。
禁止未来数据回填历史判定。
```

## 4. Adoption Manifest

```yaml
repo: xgo-macro-data
layer: L4
version: "1.0"

uses_l2:
  postgresx:
    required: true
    purpose: [metadata, release_calendar, state]
  ossx:
    required: true
    purpose: [raw_response_archive]
  clickhousex:
    required: true
    purpose: [factor_wide_table, analytics_query]
  kafkax:
    required: false
    purpose: [macro_update_event]
  redisx:
    required: false
    purpose: [current_snapshot]

required_profiles:
  - compile
  - layer
  - integration
  - information_set
  - observability
  - resilience
  - evidence

release_level: DA-T3
```

## 5. 测试矩阵

| 场景 | L2 组合 | 验收 |
|---|---|---|
| 宏观数据入库 | postgresx + ossx | 元数据与原始归档一致 |
| 因子宽表写入 | clickhousex + postgresx | batch insert 与状态更新 |
| 发布日历 | postgresx | release calendar 查询正确 |
| 宏观事件发布 | kafkax | update event 可消费 |
| 快照缓存 | redisx | current macro snapshot 可读 |
| 可观测性 | observex + all L2 | 数据源、延迟、错误完整 |

## 6. Information Set Gate

必须生成：

```text
.agent/evidence/downstream/information-set-check.json
```

检查：

```text
release_time 存在
as_of_time 存在
collected_at 存在
不使用未来数据
raw archive 可追溯
revision/vintage 字段存在或记录修订风险
```

## 7. 最小行动

```text
先确保 postgresx/ossx/clickhousex 至少 L2-T3。
再做 xgo-macro-data DA-T3。
```

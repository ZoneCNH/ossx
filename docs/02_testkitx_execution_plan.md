# testkitx 执行方案：L2 Contract Test Runtime

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`testkitx` 是 **测试运行时**，负责“怎么测”，不负责“是否允许发布”。

```text
testkitx = Contract Runner + requirex + evidence writer + service helper
xlibgate = 机器裁判
xlib-standard = 标准源
```

## 2. 目标

提供所有 L2 能力族通用 Contract Runner：

```text
common
kv
sql
pubsub
eventlog
objectstore
columnstore
timeseries
```

## 3. 目录结构

```text
requirex/
  error.go
  secret.go
  eventually.go
  equal.go
  no_error.go
  goroutine.go

contract/
  common/
    factory.go
    lifecycle.go
    config.go
    error.go
    secret.go
    resilience.go
    observability.go
  kv/
  sql/
  pubsub/
  eventlog/
  objectstore/
  columnstore/
  timeseries/

evidence/
  writer.go
  model.go

servicex/
  compose.go
  wait.go
  health.go
```

## 4. requirex 最小能力

```go
func NoError(t TestingT, err error)
func Equal[T comparable](t TestingT, want, got T)
func ErrorKind(t TestingT, err error, want string)
func ErrorKindOneOf(t TestingT, err error, wants ...string)
func NoSecretLeak(t TestingT, value any, secrets ...string)
func Eventually(t TestingT, timeout, interval time.Duration, fn func() bool)
func NoGoroutineLeak(t TestingT, before, after int)
```

## 5. Common Contract

Test IDs：

```text
common.lifecycle.start
common.lifecycle.ping
common.lifecycle.close_idempotent
common.config.invalid_config
common.error.standard_error_kind
common.secret.no_secret_leak
common.resilience.context_cancel
common.observability.metrics
```

## 6. 能力族 Runner

| 能力族 | 包 | 首批 Runner |
|---|---|---|
| KV | contract/kv | kv.go / ttl.go |
| SQL | contract/sql | sql.go / transaction.go / pool.go |
| PubSub | contract/pubsub | pubsub.go / request_reply.go |
| EventLog | contract/eventlog | producer.go / consumer.go / offset_commit.go |
| ObjectStore | contract/objectstore | objectstore.go |
| ColumnStore | contract/columnstore | columnstore.go / batch_insert.go |
| TimeSeries | contract/timeseries | timeseries.go / stable.go / child_table.go / batch_write.go |

## 7. 接口原则

```text
只定义能力接口。
不依赖具体 provider driver。
不依赖任何 L2 仓库。
L2 仓库在 test/<repo>test 中适配自己的 client。
```

## 8. PR 拆分

```text
PR-001 requirex
PR-002 common contract
PR-003 kv contract
PR-004 sql contract
PR-005 pubsub contract
PR-006 eventlog contract
PR-007 objectstore contract
PR-008 columnstore contract
PR-009 timeseries contract
PR-010 evidence writer + servicex
```

## 9. 验收标准

```text
Contract Runner 不出现 redis/postgres/kafka provider 专属实现
Test ID 与 xlib-standard registry 完全一致
Runner 只通过 interface 调用
失败输出可解释
```

## 10. 禁止事项

```text
不得连接 Redis/Kafka/Postgres
不得裁决 release
不得扫描 layer/secret
不得生成 release-readiness
不得依赖 L2 仓库
```

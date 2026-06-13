# xgo-market-data 执行方案：L4 Market Data 下游采纳门禁

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`xgo-market-data` 是 L4 数据服务层，允许组合 L2，但不能绕过 L2 直接使用 provider SDK。

## 2. 允许使用的 L2

```text
kafkax       market event bus
redisx       latest price / hot kline cache
taosx        time-series kline storage
ossx         historical archive
postgresx    metadata / cursor / collection status / configs
natsx        optional control signal
clickhousex  optional analytics query
```

## 3. 禁止

```text
禁止直接 import Kafka SDK。
禁止直接 import Redis SDK。
禁止直接 import TDengine SDK。
禁止直接 import S3 SDK。
禁止直接 import PostgreSQL driver。
禁止把 provider 原生错误暴露到业务 API。
禁止在业务代码中随意拼 Redis key / Kafka topic / TDengine table。
```

## 4. Adoption Manifest

`.agent/downstream-adoption.yaml`：

```yaml
repo: xgo-market-data
layer: L4
version: "1.0"

uses_l2:
  redisx:
    required: true
    purpose: [hot_cache, latest_price, collection_status_cache]
  kafkax:
    required: true
    purpose: [market_event_bus, kline_event_publish, validation_event_publish]
  taosx:
    required: true
    purpose: [timeseries_kline_storage]
  ossx:
    required: true
    purpose: [historical_archive, parquet_csv_archive]
  postgresx:
    required: true
    purpose: [metadata, cursor, collection_status, configs]
  natsx:
    required: false
    purpose: [control_signal]
  clickhousex:
    required: false
    purpose: [analytics_query]

required_profiles:
  - compile
  - layer
  - integration
  - observability
  - resilience
  - evidence

release_level: DA-T3
```

## 5. L2 Stack Test

```text
1. 启动 redis / redpanda / postgres / tdengine / minio
2. 模拟一条 kline event
3. kafkax 发布事件
4. taosx 写入 K 线
5. redisx 更新 latest cache
6. postgresx 更新 collection_status
7. ossx 归档历史数据
8. 校验所有状态一致
```

## 6. Evidence

```text
.agent/evidence/downstream/
  adoption-plan.json
  layer-check.json
  anti-bypass-check.json
  integration-test.json
  observability-check.json
  resilience-check.json
  l2-compatibility-check.json
  downstream-readiness.json
```

## 7. Release Gate

```text
required L2 release_ready = true
required L2 version compatible
downstream-layer-check 通过
anti-bypass-check 通过
l2-stack integration 通过
observability check 通过
resilience check 通过
evidence complete
```

## 8. 最小行动

```text
先完成 redisx/postgresx/kafkax/taosx/ossx 至少 L2-T3。
再执行 xgo-market-data DA-T3。
```

# L5 Engines 执行方案：market-engine / macro-engine / regime-engine 采纳门禁

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

L5 是引擎层，关注领域状态、因子、信号、策略和解释器日志，不应该直接操作基础设施。

```text
market-engine
macro-engine
regime-engine
```

## 2. 依赖原则

```text
优先依赖 xgo-contracts。
优先依赖 L4 API。
默认不直接依赖 L2。
禁止直接依赖 provider SDK。
```

## 3. market-engine

### 输入

```text
MarketStateInput
KlineFeatureInput
MarketSignalInput
```

### Gate

```text
不直接 import provider SDK
不直接操作 Redis/Kafka/TDengine
通过 L4 API 获取标准市场输入
错误标准化
有 fallback/degraded mode
有 explanation evidence
```

## 4. macro-engine

### 输入

```text
MacroStateInput
LGIPFactorInput
MacroReleaseInput
```

### Gate

```text
不直接访问原始 FRED/BEA/ECB provider
不直接操作 clickhouse/postgres/oss
通过 xgo-macro-data 获取标准化输入
遵守 information set
输出 macro evidence
```

## 5. regime-engine

### 特殊硬约束

```text
Regime Engine 只消费标准化状态输入。
Market Data 不直接决定 Regime。
Macro Data 不直接依赖 Market Data 内部实现。
不直接解析 provider 原始数据。
不直接依赖 L2。
```

### 输出

```text
M state
S state
MxS action
risk tier
explanation log
```

## 6. Adoption Manifest 示例

```yaml
repo: regime-engine
layer: L5
version: "1.0"

preferred_dependencies:
  - xgo-contracts
  - xgo-market-data
  - xgo-macro-data

restricted_l2:
  - redisx
  - kafkax
  - taosx
  - postgresx
  - ossx
  - clickhousex
  - natsx

forbidden_provider_imports:
  - github.com/redis/go-redis
  - github.com/segmentio/kafka-go
  - github.com/jackc/pgx
  - github.com/taosdata/driver-go

required_profiles:
  - compile
  - layer
  - contract
  - integration
  - explanation
  - evidence
```

## 7. Evidence

```text
.agent/evidence/engine/
  standard-input-contract.json
  error-handling-check.json
  fallback-check.json
  explanation-log-check.json
  engine-readiness.json
```

## 8. 验收

```text
无 provider SDK
无未授权 direct L2
标准输入 contract 通过
错误处理测试通过
fallback/degraded mode 通过
解释器日志 Evidence 存在
```

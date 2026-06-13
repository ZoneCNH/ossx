# x.go Runtime 与 System Gate 执行方案

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. x.go 定位

`x.go` 是 L6 运行时编排层，不是基础设施适配层，也不是策略逻辑大杂烩。

负责：

```text
配置装配
服务启动
生命周期管理
健康检查
可观测汇总
graceful shutdown
release runtime evidence
```

## 2. 禁止

```text
禁止直接操作 Redis / Kafka / TDengine / Postgres / OSS / ClickHouse / NATS。
禁止直接拼 Redis key / Kafka topic / TDengine table。
禁止直接处理 provider 原生错误。
禁止把基础设施逻辑写进业务主流程。
```

## 3. Runtime Manifest

`.agent/runtime-adoption.yaml`：

```yaml
repo: x.go
layer: L6
version: "1.0"

runtime_role: orchestrator

depends_on:
  L0: [kernel]
  L1: [configx, observex, resiliencx, schedulex]
  L3: [xgo-contracts]
  L4: [xgo-market-data, xgo-macro-data]
  L5: [market-engine, macro-engine, regime-engine]

restricted_l2_direct_dependency: true

required_runtime_profiles:
  - compile
  - config
  - lifecycle
  - health
  - observability
  - graceful_shutdown
  - market_stack
  - macro_stack
  - regime_stack
  - evidence

release_level: RT-T3
```

## 4. Runtime Level

| Level | 名称 | 要求 |
|---|---|---|
| RT-T0 | Skeleton Runtime | 结构和 manifest |
| RT-T1 | Compile Runtime | 编译、无 forbidden import |
| RT-T2 | Service Runtime | config / lifecycle / health |
| RT-T3 | Integrated Runtime | market/macro/engine/regime stack |
| RT-T4 | Production Runtime | chaos / SLO / rollback / production evidence |

## 5. Runtime Evidence

```text
.agent/evidence/runtime/
  raw/
  normalized/
  stack/
  decision/
    runtime-readiness.json
  trace/
    runtime-traceability-matrix.json
  retrospective.json
```

## 6. Stack Profiles

```text
minimal:
  configx / observex / health

market_stack:
  xgo-market-data / market-engine

macro_stack:
  xgo-macro-data / macro-engine

regime_stack:
  xgo-market-data / xgo-macro-data / regime-engine

full_stack:
  market-data / macro-data / all engines
```

## 7. System Gate

系统级等级：

| Level | 名称 | 要求 |
|---|---|---|
| SYS-T0 | Skeleton System | system manifest / release train |
| SYS-T1 | Compile System | 全仓库可编译 |
| SYS-T2 | Integrated System | L2/L4/L5/L6 ready |
| SYS-T3 | Staging Ready | E2E / runtime / observability / resilience |
| SYS-T4 | Production Ready | chaos / recovery / rollback / SLO / security |

## 8. system-readiness.json

```json
{
  "system": "x.go",
  "release_train": "REL-20260605-xgo",
  "environment": "staging",
  "release_level_declared": "SYS-T3",
  "release_level_actual": "SYS-T3",
  "system_ready": true,
  "production_ready": false,
  "hard_failures": {
    "secret_leak": false,
    "layer_violation": false,
    "release_train_blocked": false,
    "compatibility_failure": false,
    "runtime_failure": false,
    "data_recovery_failure": false,
    "rollback_failure": false
  },
  "evidence_complete": true
}
```

## 9. Release Train

```text
L0 kernel
  ↓
L1 configx / observex / resiliencx / schedulex / testkitx
  ↓
L2 redisx / kafkax / postgresx / taosx / ossx / clickhousex / natsx
  ↓
L3 xgo-contracts
  ↓
L4 xgo-market-data / xgo-macro-data
  ↓
L5 market-engine / macro-engine / regime-engine
  ↓
L6 x.go
```

## 10. Production Promotion

生产发布必须通过：

```text
system-readiness.json
production-promotion.json
rollback-check.json
slo-check.json
security-check.json
human-approval.json，如需要
```

## 11. 最小行动

```text
先完成 RT-T2。
再做 market_stack。
再做 macro_stack。
再做 regime_stack。
最后做 full_stack 与 SYS-T3/SYS-T4。
```

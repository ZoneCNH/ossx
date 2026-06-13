# xlib-standard 执行方案：L2 测试工厂标准源

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`xlib-standard` 是 L2 测试工厂的 **标准源 SSOT**，负责定义：

```text
Capability Manifest 规范
Contract Pack Registry
Release Level
Evidence 标准
templates/l2
downstream/runtime/system gate 文档
```

它不连接 Redis/Kafka/Postgres，不运行 provider 测试，不实现 Contract Runner。

## 2. 目标

```text
把所有 L2 适配库纳入统一标准：
声明能力 → 映射 Contract Pack → 执行 testkitx → xlibgate 裁决 → Evidence 证明。
```

## 3. 新增目录

```text
.agent/
  registry/
    l2-contract-packs.yaml
    l2-capability-families.yaml
    l2-golden-samples.yaml
    l2-release-levels.yaml
  schemas/
    l2-capabilities.schema.json
    l2-contract-packs.schema.json
    l2-release-readiness.schema.json
    l2-compliance-matrix.schema.json

templates/
  l2/
    .agent/
      l2-capabilities.yaml
      gates/l2gate.yaml
      evidence/README.md
    test/
      contract/l2_contract_test.go
      integration/README.md
      chaos/README.md
      benchmark/README.md
      adoption/README.md
    docker-compose.test.yml
    Makefile
    .github/workflows/l2-gates.yml

docs/testing/
  l2-adapter-testing-standard.md
  l2-capability-manifest.md
  l2-contract-pack-registry.md
  l2-evidence-standard.md
  l2-release-gate.md
  l2-compliance-matrix.md
  l2-rollout-playbook.md
  l2-downstream-adoption.md
  l2-compatibility-matrix.md
```

## 4. Release Level Registry

`.agent/registry/l2-release-levels.yaml`：

```yaml
version: "1.0"

levels:
  L2-T0:
    name: Skeleton Ready
    release_allowed: false
    factory_grade_allowed: false
    required_profiles: [skeleton]
    min_score: 0

  L2-T1:
    name: Contract Ready
    release_allowed: false
    factory_grade_allowed: false
    required_profiles: [unit, contract]
    min_score: 60

  L2-T2:
    name: Integration Ready
    release_allowed: false
    factory_grade_allowed: false
    required_profiles: [unit, contract, integration]
    min_score: 75

  L2-T3:
    name: Release Ready
    release_allowed: true
    factory_grade_allowed: false
    required_profiles: [unit, contract, integration, chaos, benchmark, adoption]
    min_score: 85

  L2-T4:
    name: Factory Grade
    release_allowed: true
    factory_grade_allowed: true
    required_profiles: [unit, contract, integration, chaos, benchmark, adoption, retrospective]
    min_score: 95
```

## 5. Contract Pack Registry 第一版

优先覆盖：

```text
common
kv
ttl
sql
transaction
pool
pubsub
request_reply
eventlog
producer
consumer
offset_commit
objectstore
columnstore
timeseries
```

后续扩展：

```text
pipeline / lock / stream
migration / advisory_lock / copy
queue_group / jetstream / ack / redelivery
dlq / rebalance / partition / retry
multipart / presign
ttl / partition / streaming_query / async_insert
stable / child_table / data_quality / retention
```

## 6. PR 拆分

```text
PR-001: registry + schemas + release levels
PR-002: templates/l2
PR-003: docs/testing
PR-004: golden sample registry
PR-005: downstream/runtime/system 标准
```

## 7. 验收标准

```text
所有 YAML 可解析
所有 JSON schema 可解析
templates/l2 可复制到任意 L2 仓库
docs 能解释 manifest / registry / evidence / release gate
release levels 只能由 xlib-standard 定义
```

## 8. Evidence

```text
.agent/evidence/l2-standard/schema-validate.json
.agent/evidence/l2-standard/template-check.json
.agent/evidence/l2-standard/registry-check.json
```

## 9. 禁止事项

```text
不得依赖 redisx/postgresx/kafkax 等 L2 实现
不得连接 provider
不得在标准源中写业务模型
不得让 L2 自定义自己的 Release Level
```

## 10. MVA

```text
先落地：
- l2-contract-packs.yaml
- l2-release-levels.yaml
- templates/l2/.agent/l2-capabilities.yaml
- templates/l2/Makefile
- docs/testing/l2-capability-manifest.md
```

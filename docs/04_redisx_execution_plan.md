# redisx 执行方案：KV / TTL / Lock / Stream / PubSub

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`redisx` 是 L2 基础设施适配库。目标是纳入统一 L2 测试工厂：

```text
capability manifest
  → contract pack
  → testkitx runner
  → xlibgate/l2 release-check
  → Evidence
  → release-readiness.json
```

## 2. 能力族

```text
common / kv / ttl / pipeline / lock / stream / pubsub
```

## 3. L2-T2 Capability Manifest

```yaml
repo: redisx
layer: L2
version: "1.0"

capabilities:
  common: { required: true, level: core }
  kv: { required: true, level: core }
  ttl: { required: true, level: core }
  pipeline: { required: false, level: optional }
  lock: { required: false, level: optional }
  stream: { required: false, level: optional }
  pubsub: { required: false, level: optional }

provider:
  name: redis
  test_image: redis:7-alpine

required_profiles: [unit, contract, integration]
release_level: L2-T2
```

## 4. P0 Contract Tests

```text
kv.set_get
kv.delete
kv.exists
kv.not_found
kv.validation.empty_key
kv.context_cancel
ttl.expire
ttl.not_found_after_expire
lock.no_foreign_release
stream.read_group
pubsub.publish_subscribe
chaos.redis.restart_recovery
chaos.redis.pool_exhaustion
```

## 5. 错误映射重点

```text
NOAUTH→auth
WRONGPASS→auth
nil key→not_found
context deadline→timeout
context canceled→canceled
cluster down→unavailable
pool exhausted→resource_exhausted/timeout
```

## 6. 目录结构

```text
redisx/
  .agent/
    l2-capabilities.yaml
    registry/l2-contract-packs.yaml
    gates/l2gate.yaml
    evidence/
      raw/
      normalized/
      decision/
      trace/

  test/
    contract/
      l2_contract_test.go
    integration/
    chaos/
    benchmark/
    adoption/
    redisxtest/
      factory.go
      adapter.go
      config.go

  examples/
    basic/
    with-configx/
    with-observex/
    with-resiliencx/

  docker-compose.test.yml
  Makefile
```

## 标准命令面

```bash
make l2-plan
make test-unit
make test-contract
make test-integration
make test-chaos
make test-bench
make test-adoption
make test-arch
make test-security
make evidence
make release-check
```

最小 MVA 阶段可以先保留：

```bash
make l2-plan
make test-unit
make test-contract
make test-integration
make evidence
make release-check
```


## Evidence 标准

```text
.agent/evidence/
  raw/
    unit-test.json
    contract-test.json
    integration-test.json
    chaos-test.json
    adoption-test.json
    benchmark.txt
  normalized/
    contract-check.json
    integration-check.json
    chaos-check.json
    adoption-check.json
    layer-guard.json
    secret-scan.json
  decision/
    test-plan.json
    release-readiness.json
  trace/
    traceability-matrix.json
  retrospective.json
  manifest.json
```

完成声明必须使用：

```text
DONE with evidence:
- .agent/evidence/decision/release-readiness.json
- .agent/evidence/trace/traceability-matrix.json
- .agent/evidence/retrospective.json
```


## 7. 分阶段路线

```text
L2-T2:
  common + 主能力族 + integration + release-readiness

L2-T3:
  chaos + benchmark + adoption + layer guard + secret scan

L2-T4:
  extended capabilities + traceability + retrospective + factory_grade=true
```

## 8. Rollout

```text
L2-T2 只开 common/kv/ttl。
L2-T3 增加 chaos/benchmark/adoption。
L2-T4 打开 pipeline/lock/stream/pubsub required。
```

## 9. 特殊注意

```text
Redis Lock 必须防止误删他人锁：release 必须校验 token。
Redis PubSub 不保证持久化，不要测成 Kafka EventLog。
Stream 要明确 read group / ack / pending 语义。
```

## 10. 验收标准

```text
make release-check 通过
release_level_actual 符合目标等级
hard_failures 全部 false
required_contract_tests 全部通过
required_evidence 全部存在
正式代码不依赖 testkitx
不依赖其它 L2
```

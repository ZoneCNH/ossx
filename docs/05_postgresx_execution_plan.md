# postgresx 执行方案：SQL / Transaction / Pool / Migration

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`postgresx` 是 L2 基础设施适配库。目标是纳入统一 L2 测试工厂：

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
common / sql / transaction / pool / migration / advisory_lock / batch_insert / copy
```

## 3. L2-T2 Capability Manifest

```yaml
repo: postgresx
layer: L2
version: "1.0"

capabilities:
  common: { required: true, level: core }
  sql: { required: true, level: core }
  transaction: { required: true, level: core }
  pool: { required: true, level: core }
  migration: { required: false, level: optional }
  advisory_lock: { required: false, level: optional }
  batch_insert: { required: false, level: optional }
  copy: { required: false, level: optional }

provider:
  name: postgres
  test_image: postgres:16-alpine

required_profiles: [unit, contract, integration]
release_level: L2-T2
```

## 4. P0 Contract Tests

```text
sql.exec
sql.query_row
sql.query_many
sql.not_found
sql.syntax_error
sql.unique_violation
sql.foreign_key_violation
sql.context_timeout
tx.commit
tx.rollback
tx.rollback_on_error
pool.exhaustion
```

## 5. 错误映射重点

```text
unique violation 23505→duplicate
foreign key 23503→conflict
not null 23502→validation
check 23514→validation
serialization 40001→conflict/retryable
deadlock 40P01→conflict/retryable
query timeout→timeout
pool exhausted→resource_exhausted/timeout
```

## 6. 目录结构

```text
postgresx/
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
    postgresxtest/
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
L2-T2 验证 SQL/Tx/Pool。
L2-T3 增加 restart/query timeout/benchmark/adoption。
L2-T4 打开 migration/advisory_lock/batch_insert/copy。
```

## 9. 特殊注意

```text
Postgresx 是 SQL 能力族样板，不要复制 redisx KV 语义。
Migration 要验证 dirty state 和幂等。
Tx rollback 失败或连接污染属于 P0。
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

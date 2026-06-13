# xlibgate 执行方案：机器裁判 CLI

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`xlibgate` 是基础库标准工厂的机器裁判。

```text
testkitx 负责“怎么测”
xlibgate 负责“是否合格”
xlib-standard 负责“标准是什么”
```

`l2gate` 可以作为早期 MVP，但长期应收敛为：

```text
github.com/ZoneCNH/xlibgate
```

## 2. 命令形态

```bash
xlibgate l2 validate-manifest
xlibgate l2 plan
xlibgate l2 check-contracts
xlibgate l2 check-evidence
xlibgate l2 release-check

xlibgate downstream adoption-check
xlibgate downstream anti-bypass-check

xlibgate runtime release-check
xlibgate system release-check
xlibgate system promote
```

## 3. 目录结构

```text
cmd/xlibgate/main.go

internal/
  l2/
    manifest/
    registry/
    planner/
    contracts/
    evidence/
    scoring/
    release/
  downstream/
    adoption/
    antibypass/
    layer/
  runtime/
    manifest/
    stack/
    readiness/
  system/
    releasetrain/
    readiness/
    promotion/
    rollback/
  scan/
    imports/
    secrets/
    gomod/
  report/
    json/
    markdown/
```

## 4. L2 MVP 命令

```bash
xlibgate l2 validate-manifest
xlibgate l2 plan
xlibgate l2 check-contracts
xlibgate l2 check-evidence
xlibgate l2 release-check
```

输入：

```text
.agent/l2-capabilities.yaml
.agent/registry/l2-contract-packs.yaml
.agent/evidence/raw/contract-test.json
.agent/evidence/*
```

输出：

```text
.agent/evidence/decision/test-plan.json
.agent/evidence/decision/release-readiness.json
```

## 5. 裁决顺序

```text
1. validate manifest
2. resolve test plan
3. check required contract tests
4. check required evidence
5. check layer violation
6. check secret leak
7. check race / goroutine leak
8. compute score
9. compare declared release_level
10. write release-readiness.json
```

## 6. 硬失败

```text
secret_leak
layer_violation
missing_required_contract
missing_required_evidence
race_detected
goroutine_leak
release_level_overclaimed
```

## 7. 不应该做的事

```text
不得连接 Redis/Kafka/Postgres/TDengine
不得执行 Set/Get/Query/Produce 等 provider 功能测试
不得实现 Contract Runner
不得依赖 redisx/postgresx/kafkax
```

## 8. PR 拆分

```text
PR-001 xlibgate l2 manifest/registry/planner
PR-002 xlibgate l2 check-contracts/check-evidence
PR-003 xlibgate l2 release-check/scoring
PR-004 xlibgate scan imports/secrets
PR-005 xlibgate downstream adoption/anti-bypass
PR-006 xlibgate runtime/system
```

## 9. MVA

```text
只支持 L2：
validate-manifest / plan / check-contracts / check-evidence / release-check
先跑通 redisx L2-T2。
```

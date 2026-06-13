# natsx 执行方案：PubSub / RequestReply / QueueGroup / JetStream

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。



## 1. 定位

`natsx` 是 L2 基础设施适配库。目标是纳入统一 L2 测试工厂：

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
common / pubsub / request_reply / queue_group / jetstream / ack / redelivery / dedup
```

## 3. L2-T2 Capability Manifest

```yaml
repo: natsx
layer: L2
version: "1.0"

capabilities:
  common: { required: true, level: core }
  pubsub: { required: true, level: core }
  request_reply: { required: true, level: core }
  queue_group: { required: false, level: optional }
  jetstream: { required: false, level: optional }
  ack: { required: false, level: optional }
  redelivery: { required: false, level: optional }
  dedup: { required: false, level: optional }

provider:
  name: nats
  test_image: nats:2.10-alpine

required_profiles: [unit, contract, integration]
release_level: L2-T2
```

## 4. P0 Contract Tests

```text
pubsub.publish_subscribe
pubsub.unsubscribe
pubsub.multi_subscriber
pubsub.context_cancel
request_reply.success
request_reply.timeout
request_reply.no_responder
queue_group.single_delivery
jetstream.stream_create
ack.success
redelivery.unacked_message
```

## 5. 错误映射重点

```text
connection refused→unavailable
auth failed→auth
permission violation→permission_denied
no responders→unavailable/not_found
request timeout→timeout
stream not found→not_found
connection closed→unavailable/canceled
```

## 6. 目录结构

```text
natsx/
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
    natsxtest/
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
L2-T2 只开 pubsub/request_reply。
L2-T3 打开 queue_group。
L2-T4 打开 jetstream/ack/redelivery/dedup。
```

## 9. 特殊注意

```text
NATS Core PubSub 不等于 Kafka EventLog。
JetStream 才测试持久化、ack、redelivery。
Request/Reply 必须测试 no responder 和 timeout。
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

# ossx 执行方案：ObjectStore / Aliyun OSS v1.0.1

> 文档用途：独立仓库执行方案，可直接作为 Goal / Issue / PR / Harness / Evidence 落地依据。
> 统一原则：禁止 main 直接开发；必须使用 git worktree；没有 Evidence 不允许 DONE；没有 release-readiness 不允许 Release。

## 1. 定位

`ossx` 是 L2 基础设施适配库。目标是纳入统一 L2 测试工厂：

```text
capability manifest
  → contract pack
  → testkitx runner
  → xlibgate/l2 release-check
  → Evidence
  → release-readiness.json
```

`v1.0.1` 的实际交付边界是 Aliyun OSS objectstore 主路径。S3、MinIO、Azure、GCS provider 常量保留为后续扩展点，当前实现会拒绝未实现 provider。

## 2. 能力族

```text
common / objectstore / multipart / presign / object_lifecycle
```

## 3. L2-T2 Capability Manifest

```yaml
repo: ossx
layer: L2
version: "1.0"

capabilities:
  common: { required: true, level: core }
  objectstore: { required: true, level: core }
  multipart: { required: false, level: optional }
  presign: { required: false, level: optional }
  object_lifecycle: { required: false, level: optional }

provider:
  name: aliyun_oss
  integration: real_oss_bucket

required_profiles: [unit, integration, race, vet, build]
release_level: L2-T2
```

## 4. v1.0.1 已验收场景

```text
objectstore.put_get
objectstore.delete
objectstore.list_prefix
objectstore.not_found
objectstore.metadata
objectstore.checksum
objectstore.invalid_key
objectstore.context_cancel
objectstore.pagination
objectstore.health_check
secret_safety.no_credential_persistence
```

后续 P0 不在 `v1.0.1` 范围内：

```text
multipart.cleanup_after_failure
presign.permission_boundary
provider_parity.s3
provider_parity.minio
provider_parity.azure
provider_parity.gcs
```

## 5. 错误映射重点

```text
bucket not found→bucket_not_found
object not found→not_found
access denied/auth failure→auth
invalid key→validation
upload timeout→timeout
rate limit→rate_limit
conflict→conflict
object too large→object_too_large
unknown transfer error→transfer
```

## 6. 目录结构

```text
ossx/
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
    ossxtest/
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
make test-unit
make test-race
make test-integration
make coverage
make release-check
```

真实集成测试要求调用方先通过环境变量注入配置：

```bash
OSSX_INTEGRATION=1
OSSX_ENDPOINT
OSSX_REGION
OSSX_BUCKET
OSSX_ACCESS_KEY_ID
OSSX_SECRET_ACCESS_KEY
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

`v1.0.1` 发布证据：

```text
go test ./...                                              PASS, package coverage 100.0%
go test -tags=integration ./... -covermode=atomic ...      PASS, total coverage 100.0%
go test -race -count=1 ./...                               PASS
go test -race -tags=integration ./... -run TestIntegrationAliyunOSS -count=1  PASS
go vet ./...                                               PASS
go build ./...                                             PASS
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
L2-T2 当前只开 Aliyun OSS objectstore。
L2-T3 增加 large object/chaos/benchmark/adoption。
L2-T4 打开 multipart/presign 与跨 provider parity。
```

## 9. 特殊注意

```text
对象存储不是 POSIX 文件系统。
Key 必须防路径逃逸。
Presigned URL 必须脱敏，不能进入 Evidence/Release Manifest。
```

## 10. 验收标准

```text
make release-check 通过
release_level_actual 符合目标等级
hard_failures 全部 false
真实 OSS 集成通过
单元与集成覆盖率达到 100.0%
正式代码不依赖 testkitx
不依赖其它 L2
```

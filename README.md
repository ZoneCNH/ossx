# ossx

ZoneCNH `ossx` — Aliyun OSS adapter for FoundationX. A stable BlobStore API
over Aliyun OSS with streaming semantics, multipart lifecycle, presigned URL
policy, lifecycle/retention/permission validation, retry/circuit resilience,
and observex-compatible observability hooks.

> Identity (v1.2.0): Aliyun OSS single-provider adapter (NOT a generic
> S3-compatible / multi-provider abstraction). See module/ossx/SPEC.md §1.

## 状态：v1.2.0

- ✅ 真实 Aliyun OSS adapter（`adapters/aliyun/`，SDK 隔离，FR-008）
- ✅ 流式 Put/Get（`io.Reader`/`io.ReadCloser`，不缓冲整对象，FR-004）
- ✅ 完整 Multipart 生命周期（Initiate/UploadPart/ListParts/Complete/Abort + 幂等，FR-005）
- ✅ 真实 Presign（`bucket.SignURL`，TTL ≤15min + allowlist + 审计脱敏，FR-006）
- ✅ Lifecycle/Retention/Permission 策略校验（FR-007）
- ✅ Retry + Circuit Breaker（resiliencx 语义，FR-003/005）
- ✅ observex 兼容 Hooks（Metrics/Tracer/Logger，nil-safe，FR-009）
- ✅ 三态 Health（config/unreachable/degraded，FR-010）
- ✅ 本地验收通过：pkg/ossx 覆盖率 **100.0%**；实盘 Aliyun integration 5/5 PASS（真实 bucket `x-go`：Health/PutGetDelete/List/Multipart/Presign）

## 安装

```go
import (
    "github.com/ZoneCNH/ossx/pkg/ossx"
    "github.com/ZoneCNH/ossx/adapters/aliyun"
)
```

```bash
go get github.com/ZoneCNH/ossx@v1.2.0
```

## 快速使用（Aliyun OSS）

配置从 `FOUNDATIONX_OSSX_*` 环境变量加载（组合根装配，ossx 不 import configx）：

```go
ctx := context.Background()

// 1. Load config (composition root reads secrets; ossx never imports configx).
cfg, err := ossx.ConfigFromEnv()
if err != nil { return err }

// 2. Build the real Aliyun adapter (SDK isolated here, never leaks).
adapter, err := aliyun.NewAdapter(ctx, cfg)
if err != nil { return err }

// 3. Wrap with BlobStore (adds retry/circuit/policy/hooks).
store, err := ossx.NewBlobStore(cfg, adapter, ossx.Hooks{})
if err != nil { return err }
defer store.Close(ctx)

// 4. Streaming Put (no whole-object buffering).
key, _ := ossx.NewKey("artifacts/build-001.tgz")
info, err := store.Put(ctx, key, body, ossx.PutOptions{
    ContentType:  "application/gzip",
    ChecksumAlgo: ossx.ChecksumSHA256,
})

// 5. Streaming Get (caller must Close).
reader, err := store.Get(ctx, key, ossx.GetOptions{VerifyChecksum: true})
defer reader.Close()
```

## 快速使用（in-memory，测试/示例）

```go
store, _ := ossx.NewBlobStore(cfg, ossx.NewInMemoryAdapter(), ossx.Hooks{})
// 无需 Aliyun SDK 或真实 bucket。
```

## Adapter SPI

外部 adapter 实现 `ossx.StoreAdapter`（流式签名）：

```go
type StoreAdapter interface {
    Name() string
    PutObject(ctx, key string, body io.Reader, size int64, opts PutAdapterOptions) (ObjectInfo, error)
    GetObject(ctx, key string) (io.ReadCloser, ObjectInfo, error)
    HeadObject(ctx, key string) (ObjectInfo, error)
    DeleteObject(ctx, key string, strict bool) error
    CopyObject(ctx, source, target string, opts CopyAdapterOptions) (ObjectInfo, error)
    ListObjects(ctx, prefix string, max int, continuation string) (ListPage, error)
    InitiateMultipart(ctx, key string, opts PutAdapterOptions) (UploadID, error)
    UploadPart(ctx, id UploadID, partNumber int, body io.Reader, size int64) (PartETag, error)
    ListParts(ctx, id UploadID) ([]PartETag, error)
    CompleteMultipart(ctx, id UploadID, parts []PartETag) (ObjectInfo, error)
    AbortMultipart(ctx, id UploadID) error
    PresignURL(ctx, key string, op PresignOperation, ttl int64, opts PresignAdapterOptions) (PresignedURL, error)
    Health(ctx) error
    Close(ctx) error
}
```

Provider SDK 类型必须封装在 adapter 内部（FR-008 / BR-011）。

## 集成测试

集成测试采用 build tag + 环境变量双层门禁。未设置 `OSSX_LIVE_INTEGRATION=1`
或凭证时，本地验收按设计 SKIP；真实 Aliyun OSS pass 需在凭证环境执行并归档
evidence。

```bash
# 加载 sre/secrets/env/ossx.env（gitignored，不进公开仓库）
set -a; . /home/ZoneCNH/sre/secrets/env/ossx.env; set +a
OSSX_LIVE_INTEGRATION=1 go test -tags integration ./adapters/aliyun/ -v -timeout 120s
```

## 与 SPEC 的对应

完整 SPEC：`https://github.com/ZoneCNH/ZoneCNH/blob/main/module/ossx/SPEC.md`

| FR | 状态 |
| --- | --- |
| FR-001 构造与配置校验 | ✅ |
| FR-002 Key/metadata 校验 | ✅ |
| FR-003 Put/Get/Delete/Copy/Head/Exists/List | ✅ |
| FR-004 流式上传/下载 | ✅ |
| FR-005 Multipart | ✅ |
| FR-006 Presign | ✅ |
| FR-007 Checksum/Lifecycle/Retention/Permission policy | ✅ |
| FR-008 Aliyun OSS adapter 隔离 | ✅ |
| FR-009 Hooks（observability） | ✅ |
| FR-010 Health & graceful close | ✅ |

## License

见 `LICENSE`。

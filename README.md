# ossx

ZoneCNH `ossx` — Object Storage Extension for FoundationX.

## 状态：v1.0.2-alpha

- ✅ 公开 API 全签名落地：`BlobStore` 接口（Put/Get/Delete/Copy/Head/Exists/List/Multipart/Presign/Health/Close）
- ✅ 完整类型体系：`Config` / `Key` / `Prefix` / `ObjectInfo` / `PutOptions` / 全 12 个 typed errors
- ✅ `InMemoryAdapter` 全功能内存实现，可直接用于测试与 v1.0.2 stub 集成
- ✅ `ObjectStorageAdapter` SPI（FR-008）：公开接口仅依赖 stdlib + ossx 类型
- ✅ `Hooks` 可观测性钩子（FR-009）：no-op 默认安全
- ✅ 入口校验：Config / Key / Metadata / Checksum 算法 / Presign TTL & 操作 allowlist
- ✅ `go build ./pkg/...` + `go vet` + `go test -race -count=1` 全过

### v1.0.2-alpha 明确不做（v1.1.0 准入）

- ❌ Multipart：`Multipart()` 返回的 session 全部方法 `ErrNotImplemented`
- ❌ Presign 实际签名：TTL/allowlist 校验已实现，签名 SDK 适配返回 `ErrNotImplemented`
- ❌ `adapters/s3`、`adapters/aliyun`：v1.1.0 接入

## 安装

```go
import "github.com/ZoneCNH/ossx/pkg/ossx"
```

## 快速使用（in-memory）

```go
cfg := ossx.Config{
    Endpoint:  "https://internal.example",
    Region:    "cn-hangzhou",
    Bucket:    "my-bucket",
    Timeouts:  ossx.Timeouts{Operation: 30 * time.Second},
    Multipart: ossx.MultipartPolicy{MinPartSize: 8 << 20, MaxParts: 10000},
    Presign:   ossx.PresignPolicy{MaxTTL: 5 * time.Minute, AllowedOperations: []ossx.PresignOperation{ossx.PresignGet}},
}

bs, err := ossx.NewBlobStore(cfg, ossx.NewInMemoryAdapter(), ossx.Hooks{})
if err != nil { /* handle */ }
defer bs.Close(ctx)

key, _ := ossx.NewKey("artifacts/build-001.tgz")
info, err := bs.Put(ctx, key, body, ossx.PutOptions{
    ContentType:  "application/gzip",
    ChecksumAlgo: ossx.ChecksumSHA256,
})
```

## 实现自定义 adapter

实现 `ObjectStorageAdapter` 接口（5 个方法 + Name + CloseAdapter）：

```go
type ObjectStorageAdapter interface {
    PutObject(ctx context.Context, key string, body []byte, contentType string, metadata map[string]string) (string, error)
    GetObject(ctx context.Context, key string) ([]byte, ObjectInfo, error)
    DeleteObject(ctx context.Context, key string) error
    HeadObject(ctx context.Context, key string) (ObjectInfo, error)
    ListObjects(ctx context.Context, prefix string, max int, token string) ([]ObjectInfo, string, error)
    CloseAdapter(ctx context.Context) error
    Name() string
}
```

Provider SDK 类型必须封装在 adapter 内部（FR-008 / BR-011）。

## 与 SPEC 的对应

完整 SPEC：`https://github.com/ZoneCNH/ZoneCNH/blob/main/module/ossx/SPEC.md`

| FR | 状态 |
| --- | --- |
| FR-001 构造与配置校验 | ✅ |
| FR-002 Key/metadata 校验 | ✅ |
| FR-003 Put/Get/Delete/Copy/Head/Exists/List | ✅ |
| FR-004 流式上传/下载 | ✅（基础路径；超大对象分片由 multipart 兑现） |
| FR-005 Multipart | ⚠️ ErrNotImplemented（v1.1.0） |
| FR-006 Presign | ⚠️ TTL/allowlist 已校验，签名 v1.1.0 |
| FR-007 Checksum/Lifecycle/Permission policy | ✅ checksum；其余 v1.1.0 |
| FR-008 Adapter SPI | ✅ |
| FR-009 Hooks（observability） | ✅ |
| FR-010 Health & graceful close | ✅ |

## License

见 `LICENSE`。

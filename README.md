# ossx

`ossx` 是 ZoneCNH L2 对象存储适配库。

`v1.0.1` 已基于真实阿里云 OSS 环境验证 objectstore 主路径。`S3`、`MinIO`、`Azure`、`GCS` provider 常量目前仅作为后续扩展边界保留，客户端会明确拒绝未实现 provider，避免误宣称支持。

## Go 模块

```text
module github.com/ZoneCNH/ossx
```

## 当前状态

| 项目 | 状态 |
| --- | --- |
| 版本 | `v1.0.1` |
| 已验证 provider | Aliyun OSS |
| 核心能力 | put / get / delete / list / metadata / health / error mapping |
| 覆盖率 | `go test -tags=integration ./... -covermode=atomic -coverprofile=coverage.out` 总覆盖率 100.0% |
| 非目标能力 | multipart、presign、跨 provider parity |

## 使用示例

```go
client, err := ossx.New(ossx.Config{
	Name:            "default",
	Provider:        ossx.ProviderAliyunOSS,
	Bucket:          "example-bucket",
	Endpoint:        "oss-ap-northeast-1.aliyuncs.com",
	Region:          "ap-northeast-1",
	AccessKeyID:     os.Getenv("OSSX_ACCESS_KEY_ID"),
	SecretAccessKey: os.Getenv("OSSX_SECRET_ACCESS_KEY"),
})
if err != nil {
	return err
}
defer client.Close()

err = client.PutObject(ctx, "path/object.txt", strings.NewReader("hello"), ossx.PutInput{
	ContentType: "text/plain",
})
```

## 验证命令

```bash
go test ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

真实 OSS 集成测试需要调用方自行注入环境变量，不要把凭证写入仓库：

必需变量：

- `OSSX_INTEGRATION`：设为 `1`
- `OSSX_ENDPOINT`
- `OSSX_REGION`
- `OSSX_BUCKET`
- `OSSX_ACCESS_KEY_ID`
- `OSSX_SECRET_ACCESS_KEY`

```bash
go test -tags=integration ./... -run TestIntegrationAliyunOSS -count=1
go test -tags=integration ./... -covermode=atomic -coverprofile=coverage.out
```

## 发布证据

`v1.0.1` 发布前已完成：

- `go test ./...`，包级覆盖率均为 100.0%
- `go test -tags=integration ./... -covermode=atomic -coverprofile=coverage.out`，总覆盖率 100.0%
- `go test -race -count=1 ./...`
- `go test -race -tags=integration ./... -run TestIntegrationAliyunOSS -count=1`
- `go vet ./...`
- `go build ./...`

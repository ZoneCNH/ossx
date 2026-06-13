# ossx

对象存储 SDK，继承 xlib-standard 规范。

## 发布边界

- `v1.0.1` 已用真实阿里云 OSS 配置验证 objectstore 主路径。
- 当前可声明支持的 provider 是 Aliyun OSS。
- `S3`、`MinIO`、`Azure`、`GCS` 仅保留为后续 provider 常量；没有真实集成证据前不得写成已支持。
- multipart、presign 与跨 provider parity 是后续任务，不属于当前版本完成范围。

## 构建与测试
- 单元: `go test ./...`
- race: `go test -race -count=1 ./...`
- vet: `go vet ./...`
- 构建: `go build ./...`
- 集成: 设置 `OSSX_INTEGRATION=1` 与 OSS 环境变量后运行 `go test -tags=integration ./... -run TestIntegrationAliyunOSS -count=1`

## 安全

真实 OSS 配置只能通过环境变量进入测试进程。不要提交、打印或写入 AccessKey、Secret、账号 ID、私有端点和临时运行态。

# ossx Agent 指南

## 模块结构
- `pkg/ossx/` — 公共 API、provider 适配、健康检查、错误映射与指标接口
- `internal/` — 私有校验与脱敏工具
- `docs/` — L2 执行方案与发布边界说明

## 当前发布边界
- `v1.0.1` 只声明 Aliyun OSS objectstore 主路径已通过真实集成验证。
- `S3`、`MinIO`、`Azure`、`GCS` 常量为预留扩展点；没有 provider-specific 集成证据前不得写成已支持。
- multipart、presign、跨 provider parity 不属于 `v1.0.1` 完成范围。

## 验证要求
- 本地基础验证：`go test ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`。
- 覆盖率验证：`go test -tags=integration ./... -covermode=atomic -coverprofile=coverage.out`。
- 真实 OSS 集成只通过环境变量注入配置；不要输出、记录或提交凭证。

## 安全规则
- 不提交 AccessKey、Secret、私有端点、账号 ID 或临时运行态目录。
- `.omx/`、`*.out`、`coverage.html`、`dist/` 必须保持未提交。

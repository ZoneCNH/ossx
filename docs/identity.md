# ossx 身份

## 我是谁

`ossx` 是 FoundationX 的 **对象存储扩展模块**，提供 Aliyun OSS 单 provider 对象存储客户端封装。它不是 S3-compatible / 多 provider 抽象。

## 我做什么

- 对象存储 Put/Get/Delete/List 接口封装
- 分片上传和断点续传
- 签名 URL 生成

## 我不做什么

- 不是文件业务逻辑 — 存储语义由调用方定义
- 不是 S3-compatible 通用对象存储抽象 — 只承诺 Aliyun OSS adapter
- 不是模板源 — 模板生成属于 xlib-standard
- 不依赖其他存储模块

## 宪法合规

| 条款 | 遵循方式 |
|------|----------|
| §3.3 | 存储扩展，可依赖 kernel + observex (interface-only) |
| §3.4 | 不依赖 configx、业务域、其他存储扩展 |
| §1 P13 | 存储扩展之间平级协作 |

## 改了什么

一句话说明动机与结果（不是罗列文件）。

## 影响面

- 层：框架壳层 / 安全容器 / 系统集成 / IPC / freedom-cli / 构建打包 / SDK / 文档
- 平台：win-x64 / darwin-arm64 / linux-x64
- 是否改公共 API、IPC 协议、FRDM3 容器参数：是 / 否（改协议或容器参数必须说明兼容性与迁移代价）

## 验证

贴命令与真实输出（不要写"已验证"就完事）：

```
$ go vet -unsafeptr=false ./...
$ go test ./...
$ node --test tests/*.test.mjs
```

- [ ] 跨平台改动已在受影响平台分别跑过（Linux 请注明发行版与 WebKitGTK 4.0/4.1）
- [ ] bug 修复附带能复现原问题的回归测试
- [ ] `freedom-cli/templates/go` 镜像已同步（CI `Template mirror in sync` 会拦）

## 遗留与已知不足

明确写出没做的部分与原因；涉及版本号请留空由维护者决定。

## 安全

- [ ] 未提交密钥、token、私有源码或用户数据（含 `~/.npmrc`、签名私钥、`FREEDOM_GITHUB_TOKEN`）
- [ ] 未新增/自增版本号（版本纪律见 CONTRIBUTING）

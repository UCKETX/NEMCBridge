# 参与开发

欢迎报告可复现问题、补充文档和提交协议适配。大范围改动建议先开 Issue 说明目标、支持版本及验证方式。

## 开发环境

- 使用 `go.mod` 中指定的工具链，当前为 Go 1.26.9。
- 发布打包脚本需要 Python 3.11 或更新版本。
- `vendor-src/nemc-tan-lobby-solver` 是本地修改的模块，须保留，不要直接改回远程依赖。

```sh
go test ./...
go vet ./...
go test github.com/Happy2018new/nemc-tan-lobby-solver/core/sctp -run 'TestChecksumWireCompatibility|TestPacketUnmarshal|TestPacketMarshal'
go build -trimpath -o NEMCBridge .
```

Windows 用 `-o NEMCBridge.exe`，或运行 `scripts/build.ps1`。构建前请正常退出同路径下正在运行的可执行文件。

## 代码组织

| 目录 | 职责 |
| --- | --- |
| `internal/auth`、`internal/room` | FGAuth 合约、开房和信令 |
| `internal/bridge`、`internal/relay`、`internal/gamebatch` | BDS 加密连接、转发及批次边界 |
| `internal/netease` | 世界、玩家、皮肤、聊天与输入协议转换 |
| `internal/config`、`internal/secret`、`internal/session` | 配置、凭据存储及账号会话互斥 |
| `internal/integration` | 可选真实 BDS 与 WebRTC 集成测试 |
| `web` | 嵌入程序的控制台资源 |

## 真实 BDS 测试

在独立测试世界中运行 [部署指南](docs/setup.md) 对应版本的 BDS，再设置：

```sh
LOBBYKEEPER_BDS_TEST=127.0.0.1:19132 go test ./internal/integration -count=1
```

PowerShell：

```powershell
$env:LOBBYKEEPER_BDS_TEST = '127.0.0.1:19132'
go test ./internal/integration -count=1
Remove-Item Env:LOBBYKEEPER_BDS_TEST
```

没有设置该变量时，相关 BDS 测试会跳过；本地 WebRTC 及普通单元测试仍按各自条件运行。资源包下载测试另使用 `LOBBYKEEPER_BDS_RP_TEST`，需要单独配置带测试资源包的 BDS。通过协议测试后，仍应使用真实客户端确认游戏表现。

Windows 本地辅助脚本：`scripts/download-bedrock.py` 下载指定版本的官方测试服，`scripts/start-bedrock.ps1` 将其配置为创造、和平和离线模式。它们用于测试，会修改测试服的 `server.properties`。开发环境可直接调用 `scripts/start.ps1` 和 `scripts/stop.ps1`；启动脚本前台运行以显示初始管理密码，停止脚本会要求登录密码（或读取 `NEMCBRIDGE_ADMIN_PASSWORD`）。脚本使用 `current/` 数据目录和 18323 控制台端口；发布版直接启动程序默认使用 18321。

## 提交要求

- Go 代码运行 `gofmt`，协议改动提供有意义的格式或往返验证。
- 注明协议来源、字段推断与真实客户端验证的边界，参考 [协议说明](docs/protocol.md)。
- 不提交令牌、配置、世界、日志、玩家身份数据、个人路径或私有服务凭据。协议 fixture 应脱敏。
- PowerShell 脚本保留 UTF-8 BOM 和 CRLF，以兼容 Windows PowerShell 5.1；CMD 保留 CRLF。
- 保留第三方版权与许可证；贡献原创代码时同意按本项目 MIT 许可证提供该贡献。

## 发布流程

`.github/workflows/release.yml` 在 main 推送和 Pull Request 上执行三平台测试及六平台架构构建。推送新的 `vX.Y.Z` 标签后，所有检查成功才发布 Release。已有标签不移动。

维护者应先添加 `docs/releases/vX.Y.Z.md`，再提交并推送对应标签。工作流会用该文件作为说明，并打包程序、使用文档、配置示例和 SHA256；无需提供机器人令牌或 FGAuth Secrets。

维护者可通过 workflow_dispatch 对选定提交构建；填写的已有标签必须指向该提交。发布权限只赋予 release job。

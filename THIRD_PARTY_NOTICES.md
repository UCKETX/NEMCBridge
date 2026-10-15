# 第三方组件与归属

## 本项目的许可范围

根目录 [MIT License](LICENSE) 适用于 NEMCBridge 的原创代码和文档。该声明不会重新授权 `vendor-src/` 中的上游代码、其他依赖、第三方商标或官方游戏资源。

## 本地保留的上游模块

本项目通过 go.mod 的 replace 使用 `github.com/Happy2018new/nemc-tan-lobby-solver` v0.2.8，本地源码位于 `vendor-src/nemc-tan-lobby-solver`。本地修改涉及房间版本、取消与关闭处理、数据首包时序、SCTP 校验和与 NetherNet 大消息分片。

| 组件目录 | 保留的许可证 |
| --- | --- |
| `minecraft`、`protocol` | MIT，Sandertv |
| `core/nethernet` | MIT，Dragonfly Tech |
| `core/raknet` | MIT，以文件原文为准 |
| `core/datachannel`、`core/sctp`、`core/webrtc` | MIT，以各自 LICENSE、LICENSES 和文件头为准 |

**上游模块的整体许可尚待确认。** 当前导入的快照没有模块根 LICENSE，部分房间和鉴权实现没有独立许可声明。上述子组件的 MIT 文件不能自动覆盖这些代码；本项目也不对这些部分补发许可。公开分发前，维护者需要取得上游的明确许可或其他可核对的授权依据。不要将整个仓库理解为全部由本项目 MIT 许可覆盖。

WebRTC 上游测试含用于 PEM/证书解析测试的固定测试密钥；它们不是 NEMCBridge 的运行凭据，不应被用于任何真实部署。

## Go 依赖及发布包

依赖版本由 `go.mod` / `go.sum` 固定，包括 gophertunnel、go-raknet、Pion 系列及其他模块。源码保留所附的原始许可证和声明，Go 工具链和通过 Go 模块下载的依赖许可见各自上游源码。

发布压缩包不附带许可证或第三方声明文件；使用文档中的相关链接指向本仓库。第三方代码的许可范围仍以其原始声明为准。

## 协议参考与游戏资源

协议布局参考 Eulogist 4.8.0 的静态比较、neomega-core 的包结构和 neomega-admin 的皮肤处理方式。来源及推断边界见 [协议说明](docs/protocol.md)。本仓库没有打包 Eulogist 应用源码或程序。

官方 Bedrock Dedicated Server、游戏客户端、测试世界及第三方资源包需要使用者自行取得，本仓库和 Release 不包含这些内容。Minecraft、Mojang、Microsoft 和网易的名称及商标归各自权利人所有；本项目不代表这些权利人。

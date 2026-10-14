# 第三方组件

本项目包含修改后的 `github.com/Happy2018new/nemc-tan-lobby-solver` v0.2.8，本地源码位于 `vendor-src/nemc-tan-lobby-solver`，通过 go.mod 的 replace 使用。

其中 Minecraft/RakNet、NetherNet、WebRTC、SCTP、DataChannel 等组件的原始许可证保留在各自源码目录。修改包括房间版本、取消及关闭处理、数据首包接收时序和 SCTP 校验兼容。

其余 Go 依赖的版本由 go.mod/go.sum 固定。Release 的 `licenses/` 目录包含依赖模块的许可证、声明及 Go 工具链许可证，`dependencies.txt` 列出模块版本。

协议布局参考用户提供的 Eulogist 4.8.0 原始程序的静态分析；相关地址及验证边界见 `协议参考记录.md`。本仓库未包含 Eulogist 的程序、应用源码、个人配置或令牌。

官方 Bedrock Dedicated Server 及测试世界由用户自行部署，不包含在源码仓库或 Release 中。

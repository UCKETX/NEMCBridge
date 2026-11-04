# 第三方组件与归属

NEMCBridge 程序包含下列第三方组件，各组件保留其原有许可。从 v0.1.9 起，每个发布包附带的 `THIRD_PARTY_LICENSES.md` 收录了该平台程序实际编译进的全部第三方模块及其许可证原文。

## 主要组件

| 组件 | 用途 | 许可 |
| --- | --- | --- |
| [nemc-tan-lobby-solver](https://github.com/Happy2018new/nemc-tan-lobby-solver) v0.2.8（含本地修改） | 网易本地联机房间、NetherNet / WebRTC 传输 | 见下文 |
| [gophertunnel](https://github.com/Sandertv/gophertunnel) | Bedrock 协议 | MIT |
| [go-nethernet](https://github.com/df-mc/go-nethernet) v1.0.26 | 新版 BDS NetherNet 信令和传输 | MIT |
| [go-raknet](https://github.com/Sandertv/go-raknet) | RakNet 连接 | MIT |
| [GeyserMC/Geyser](https://github.com/GeyserMC/Geyser) `dbed595` | 协议 860 原版顺序方块 ID 与哈希 ID 对照数据 | MIT，见 `THIRD_PARTY_LICENSES.md` |
| [Pion](https://github.com/pion) | WebRTC、DTLS、ICE、SCTP 等 | MIT |
| Go 扩展库（`golang.org/x/crypto`、`net`、`sys`） | 加密与系统接口 | BSD-3-Clause |
| 其他依赖 | 压缩、编码、UUID 等 | 见 `THIRD_PARTY_LICENSES.md` |

## nemc-tan-lobby-solver

本项目使用 `github.com/Happy2018new/nemc-tan-lobby-solver` v0.2.8，并在房间版本、取消与关闭处理、数据首包时序、SCTP 校验和及 NetherNet 大消息分片方面做了本地修改。其子组件的许可如下：

| 组件目录 | 许可 |
| --- | --- |
| `minecraft`、`protocol` | MIT，Sandertv |
| `core/nethernet` | MIT，Dragonfly Tech |
| `core/raknet` | MIT，以文件原文为准 |
| `core/datachannel`、`core/sctp`、`core/webrtc` | MIT，以各自 LICENSE 和文件头为准 |

**上游模块的整体许可尚待确认。** 该模块没有根目录许可证，部分房间和鉴权实现（如 `bunker`）没有独立的许可声明；上述子组件的 MIT 许可不能自动覆盖这些代码。

## 协议参考与游戏资源

协议布局参考了 Eulogist 4.8.0 的静态比较、neomega-core 的包结构和 [neomega-admin](https://github.com/UCKETX/neomega-admin) 的皮肤处理方式；NEMCBridge 不包含 Eulogist 的源码或程序。

官方 Bedrock Dedicated Server、游戏客户端、世界及第三方资源包需要使用者自行取得，发布包不包含这些内容。Minecraft、Mojang、Microsoft 和网易的名称及商标归各自权利人所有；本项目不代表这些权利人。

26.30–26.60 的独立协议编解码位于 `internal/bedrock/schemas/`，由 Sandertv/gophertunnel 的固定提交生成，各目录保留原 MIT 许可证；版本及提交见 `scripts/generate_protocol_schemas.py`。新版内容映射另使用 GeyserMC/Geyser `64bb3993b4beef6743af0ab76ee5dfc8e67a0d21` 的方块及物品注册表，遵循其 MIT 许可。

0.8.7 白杨树苗、红色灌木资源继续采用固定的 Mojang/bedrock-samples 提交
`15701fa9c13923f12662984c32c124327e40e3d3`；文件及哈希记录于
`addons/netease_direct_content/poplar-plant-sources.json`。不改变原资源授权条款。

独立方块挖掘属性核对使用 PrismarineJS/minecraft-data 的 Bedrock 1.26.60 数据，
固定提交 `bd8b5c6048e5af9694374b83f25f5b4aade619cc`，上游 README 声明 MIT。
提取范围与源文件 SHA-256 记录于 `addons/netease_direct_content/mining-sources.json`。

### Poplar door item sprite / 0.8.9

The official poplar door item sprite is reused from Mojang/bedrock-samples commit
`15701fa9c13923f12662984c32c124327e40e3d3`, under the repository's existing asset
license terms. Exact source hash: `addons/netease_direct_content/poplar-item-sources.json`.
Collision serialization was checked against Geyser commit
`b728af2e89d77d4d0f55bbc90af10ba612b70a03` (MIT); no Java code was copied.

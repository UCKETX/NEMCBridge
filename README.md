<p align="center">
  <img src="docs/assets/nemcbridge-banner.png" alt="NEMCBridge：下界传送门连接网易版与国际版 Minecraft，玩家在同一个世界相遇" width="960">
</p>

<h1 align="center">NEMCBridge</h1>

<p align="center">让网易版与国际版 Bedrock 玩家，在同一个世界相遇。</p>

<p align="center">
  <a href="https://github.com/UCKETX/NEMCBridge/releases/latest">下载版本</a> ·
  <a href="docs/setup.md">部署指南</a> ·
  <a href="docs/compatibility.md">兼容范围</a> ·
  <a href="CONTRIBUTING.md">参与开发</a>
</p>

<p align="center">
  <a href="https://github.com/UCKETX/NEMCBridge/actions/workflows/release.yml"><img src="https://img.shields.io/badge/Actions-%E6%9F%A5%E7%9C%8B%E6%9E%84%E5%BB%BA-2088ff?logo=githubactions&logoColor=white" alt="查看 Actions 构建"></a>
  <a href="https://github.com/UCKETX/NEMCBridge/releases/latest"><img src="https://img.shields.io/badge/Release-%E4%B8%8B%E8%BD%BD%E7%89%88%E6%9C%AC-8b5cf6?logo=github&logoColor=white" alt="下载发布版本"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/original_code-MIT-22c55e" alt="原创代码采用 MIT 许可证"></a>
</p>

## 它能做什么

NEMCBridge 通过 FGAuth 登录房主机器人，持续创建网易《我的世界》的「本地联机」房间，再将玩家连接转发到已有的 Bedrock 服务器。网易玩家从房间加入，国际版 Bedrock 玩家直接连接同一台服务器。

- **共同游玩**：区块加载、方块破坏与放置、双向文字聊天和玩家皮肤互显。
- **房主常驻**：机器人保持房间连接，掉线后自动重试开房；重新开房时房间号可能变化。
- **网页管理**：内置 Web 控制台，配置验证服务器、令牌、后端地址和房间参数，查看连接状态与日志。
- **跨平台部署**：Windows、Linux、macOS，分别提供 amd64 与 arm64 发布包。
- **本地管理凭据**：Windows 使用 DPAPI 加密；Linux/macOS 以 `0600` 权限保存令牌。

```mermaid
flowchart LR
    N[网易版玩家] --> R[本地联机房间]
    R --> L["NEMCBridge<br/>WebRTC ↔ RakNet · 协议转换"]
    L --> B["Bedrock 服务器<br/>同一个世界"]
    I[国际版 Bedrock 玩家] --> B
    F[FGAuth] -. 房主鉴权 .-> L
```

## 快速开始

### 1. 准备后端

运行自己的 Bedrock Dedicated Server（BDS）。已验证版本为 **1.21.124.2**，在 `server.properties` 中设置：

```properties
online-mode=false
compression-algorithm=zlib
block-network-ids-are-hashes=true
```

挂服机需要将玩家转为后端的离线身份，因此后端关闭 Xbox 正版登录校验；玩家身份不应被当作已认证的 Xbox 身份。请按自己的访问范围配置后端防火墙及权限。服务器程序和世界需要自行准备。

### 2. 下载并启动

从 [Releases](https://github.com/UCKETX/NEMCBridge/releases/latest) 下载对应平台和架构：

| 系统 | 文件 | 常见架构 |
| --- | --- | --- |
| Windows | `windows_*.zip` | 大多数 PC 选 `amd64`，ARM 设备选 `arm64` |
| Linux | `linux_*.tar.gz` | x86 服务器选 `amd64`，ARM 服务器选 `arm64` |
| macOS | `macos_*.tar.gz` | Apple Silicon 选 `arm64`，Intel 选 `amd64` |

项目已由 LobbyKeeper 更名为 NEMCBridge；v0.1.2 及更早的发布包仍使用 `LobbyKeeper` 文件名，下列命令适用于新版或源码构建。

Windows 解压后双击 `NEMCBridge.exe`。Linux/macOS 在解压目录执行：

```sh
chmod +x NEMCBridge
./NEMCBridge -data-dir ./data
```

打开 **http://127.0.0.1:18321/**，填写 [FGAuth 验证服务器](https://fgauth.ucket.top)、自己的机器人令牌和后端地址，保存并点击「开房」。后端在同一台机器时，默认地址为 `127.0.0.1:19132`。

### 3. 一起加入

网易玩家在「本地联机」中搜索控制台显示的房间号；国际版 Bedrock 玩家通过服务器地址和 UDP 端口加入同一个 BDS。让机器人账号只保留一个登录会话。

更多配置、远程管理、升级及常见问题见 [部署指南](docs/setup.md)。

## 兼容范围

| 组件 | 已验证组合 |
| --- | --- |
| 网易客户端 | `3.9.41.300665` |
| 网易房间版本 / 协议 | `1.21.120.0` / `42` |
| BDS | `1.21.124.2`，游戏协议 `860` |
| 国际版客户端 | Bedrock `1.21.124` |

Windows 环境已完成真实客户端互通验证；Actions 覆盖三个系统的自动测试和六个平台架构的构建。其他游戏版本、特殊皮肤及模组内容需要另行验证。

网易端目前**跳过服务端资源包和行为包**，使用客户端已有资源。依赖这些包的自定义内容可能无法正常显示；国际版直连的资源包流程不受影响。Java 版客户端不在支持范围内。

完整边界见 [兼容性说明](docs/compatibility.md)。

## 从源码构建

使用 `go.mod` 指定的 Go 工具链；当前为 Go `1.26.9`。

```sh
git clone https://github.com/UCKETX/NEMCBridge.git
cd NEMCBridge
go test ./...
go build -trimpath -o NEMCBridge .
./NEMCBridge -data-dir ./data
```

Windows 可执行 `powershell -ExecutionPolicy Bypass -File scripts/build.ps1` 构建 `NEMCBridge.exe`。

```text
.github/       测试、构建、发布和协作模板
docs/          部署、兼容性、协议说明及图片
internal/      鉴权、房间、转发、协议转换和数据存储
scripts/       构建、打包及本地测试工具
vendor-src/    保留本地修改的上游协议模块
web/           嵌入可执行文件的 Web 控制台
```

开发与集成测试见 [CONTRIBUTING.md](CONTRIBUTING.md)，版本变化见 [CHANGELOG.md](CHANGELOG.md)。

## 致谢与许可

感谢 [nemc-tan-lobby-solver](https://github.com/Happy2018new/nemc-tan-lobby-solver)、[gophertunnel](https://github.com/Sandertv/gophertunnel)、[Pion](https://github.com/pion)、[neomega-admin](https://github.com/UCKETX/neomega-admin) 及相关协议研究。协议适配参考与实现边界见 [协议说明](docs/protocol.md)。

NEMCBridge 原创代码和文档采用 [MIT License](LICENSE)。`vendor-src/` 中的第三方代码不因本项目的 MIT 声明而重新授权；各组件的许可和上游许可缺失情况见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

本项目为独立社区工具，与 Mojang、Microsoft 或网易无隶属、赞助或官方合作关系。Minecraft 及相关商标归各自权利人所有。

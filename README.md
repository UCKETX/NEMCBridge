# 本地联机挂服机 LobbyKeeper

通过 FGAuth 登录房主机器人、创建网易“本地联机”房间，并把玩家的 WebRTC 数据转发至 Bedrock UDP 服务器。支持 Windows、Linux、macOS；机器人持续连接房间节点，掉线后自动重开房间，重开后的房间号可能变化。

## 下载发布版

GitHub 私人仓库的 [Releases](https://github.com/UCKETX/LobbyKeeper/releases) 提供 Linux、Windows、macOS 的 amd64 和 arm64 包及 SHA256 校验文件。下载后按包内 README 配置自己的后端和令牌。Web 控制台已嵌入程序；官方 BDS 和世界需自行部署。

通用启动方式及平台存储差异见 [发布版说明](RELEASE_README.md)。Windows 实机已验证游戏互通；其他平台由 Actions 验证编译和自动测试。

## 当前测试环境

- 网易客户端：3.9.41.300665。
- 房间内核版本字段：1.21.120.0；房间协议 42。
- 官方 Bedrock Dedicated Server：1.21.124.2，MOTD 1.21.124，游戏协议 860。
- 后端：127.0.0.1:19132。
- 当前配置和加密凭据目录：`current`。
- 当前调试控制台：http://127.0.0.1:18323/。

**状态：实机验证通过。** 2026-10-15，网易 3.9.41.300665 已通过房间 899359 正常进入本地 1.21.124 世界；用户确认方块破坏、放置和区块加载正常。当前机器人继续保活，后端服务器继续运行。

## 启动与停止

双击 `启动挂服机.cmd`。脚本检查控制台是否已经运行，必要时启动现有的本地测试服，然后启动挂服机。当前最新程序是 `LobbyKeeper-worldinit.exe`，配置目录为 `current`。控制台显示房间号、后端状态、玩家数据通道数量和连接阶段日志。

双击 `停止挂服机.cmd` 正常关闭房间及挂服机进程。Bedrock 测试服继续运行，世界文件位于 `runtime/bedrock-1.21.124.2/worlds`。

在网页停止房间后可以修改目标服务器地址、房间密码、人数和重连间隔。目标可改成已有的 Bedrock 服务器。房间内核版本字段应与网易客户端匹配，与后端 MOTD 是独立信息。

## 凭据与日志

Windows 令牌保存为 DPAPI 加密文件，绑定当前用户；Linux/macOS 使用仅当前用户可读写的 0600 文件权限。Windows 本地启动脚本的文件位于 `current/secrets.dat`；发布版默认位于指定数据目录。网页中令牌输入框留空会保留原令牌。控制台只监听本机，写入接口校验同源请求。

日志：`current/logs/keeper.log`。日志记录连接阶段，不输出令牌、SDP 原文、TURN 密码或玩家登录 JWT。`current/logs/status.json` 为状态快照。

测试服使用创造、和平、关闭正版登录验证的配置，用于验证网易客户端转发。**后端需 online-mode=false，使用默认 Flate 压缩、block-network-ids-are-hashes=true**：挂服机验证玩家证书和皮肤的签名，将名称、UUID、皮肤转换为后端的本地离线身份；后端 XUID 清空，不代表 Xbox 认证。每次连接使用新密钥完成 BDS 游戏加密；玩家至挂服机使用 WebRTC 的 DTLS 加密。

当前世界初始化转换针对游戏协议 860：保留哈希方块网络 ID，将 StartGame 的整张方块表校验和置零，关闭客户端地形生成，并补齐网易要求的 32 个世界设置字段（默认编码 50 字节）。字段布局及默认值参照用户提供的 Eulogist 4.8.0 原始程序的静态分析；具体证据见 `协议参考记录.md`。指定网易客户端已实测世界加载和方块交互正常。新协议版本需要另行适配，后端的国际版专属内容也需客户端支持。

资源包栈补齐网易的两个空数组；每条生物群系定义在公共固定字段后补齐 int32 和 string，保留标签及名称字典，移除客户端地形生成参数。生物群系解码使用协议 860 对应的 gophertunnel v1.51.1；转换边界和异常输入均有验证。

新版还补齐玩家资料、能力、配方和背包界面选项扩展，转换移动输入标志及物品操作的容器编号。修改后的世界初始化数据保留 Flate 压缩。兼容验证的具体范围和剩余项见 `验证记录.md` 与 `协议参考记录.md`。

官方 BDS 监听本机可用网卡的 UDP 19132/19133，挂服机自身的控制台只监听 127.0.0.1。

## 构建与验证

需要 Go 工具链（本项目使用 go1.26.9）。运行 `构建.ps1`。

Linux/macOS 源码构建：`go build -trimpath -o LobbyKeeper .`，然后 `./LobbyKeeper -data-dir ./data`。源码运行不要求仓库含本地测试服。

自动测试覆盖鉴权合约、DPAPI、包边界、取消关闭、房间 Tips 格式、标准与旧版 SCTP 校验及网易初始化/输入转换。真实本地 WebRTC → RakNet → BDS 测试通过环境变量 `LOBBYKEEPER_BDS_TEST=127.0.0.1:19132` 启用；它验证登录、资源包交换、完整初始化批次及实际地形子区块传输，另测试有缓存时的缺失区块数据响应。这些测试不能替代网易客户端进世界和交互验证。

协议库在 `vendor-src/nemc-tan-lobby-solver`，由 go.mod 的 replace 引用。本项目修复涉及房间 GameVersion、关闭竞态、信令取消、数据首包接收时序和 SCTP 校验格式。各依赖许可证保留在其源码目录。

## Actions 构建与发布

工作流位于 `.github/workflows/release.yml`，无需配置机器人令牌或 FGAuth Secrets。

- 推送 main 或创建 Pull Request：三个系统分别运行自动测试、go vet、SCTP 校验测试和本机二进制检查，同时构建六个系统/架构压缩包，保存为 Actions Artifacts。
- 推送 `v` 开头的版本标签（如 `v0.1.1`）：全部测试和构建成功后，自动在私人仓库发布 Release。
- Actions → Build and Release → Run workflow：版本标签留空仅构建，填写 `v0.1.1` 发布选定提交；已有标签必须对应选定提交，不能移动已有版本标签。

```sh
git tag v0.1.1
git push origin v0.1.1
```

发布 job 才具有 `contents: write` 权限。第三方 Actions 固定为经过核对的 commit SHA。包中附依赖许可证和模块版本清单，排除配置、令牌、日志、服务器和世界数据。Release 继承仓库的私人可见性。

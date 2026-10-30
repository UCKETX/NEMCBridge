# 网易 3.10 / Bedrock 1.21.130 适配

更新日期：2026-10-30。

## 目标与结果

| 项目 | 当前值 |
| --- | --- |
| 用户提供的 Android 安装包 | `3.10.5.301327`，versionCode `840301327` |
| 包名 | `com.netease.x19` |
| 网易房间内核版本 | `1.21.130.0` |
| 国际版 | `1.21.130`，游戏协议 `898` |
| 官方 BDS 测试版本 | `1.21.130.4` |
| 标准协议库 | `gophertunnel v1.52.2` |
| 房间控制协议 | FGAuth 依据认证引擎及官方映射返回，不与游戏协议混用 |

已完成源码、配置迁移和官方 BDS 的协议集成测试；本地 FGAuth 与桥接已启动，真实网易客户端已进服并发送聊天。实机随后反馈挥手掉线，定位为漏转 AnimatePacket，已补充双向转换及回归测试。用户随后确认第二版实机连续挥手、挖掘正常。完整渲染与其他交互验收尚未完成。

## 最新安装包核对

最初使用本机 `3.10.0.300878` 作对照，随后按用户提供的 `3.10.5.301327` 安装包重新检查。下列结论绑定最新安装包，不使用旧包结论代替新包证据。

- APK SHA256：`8fcdd06ade33e3120150b7f6acef8cf77aca0bee9e00e43a0f7859d1eef42b8e`。
- ARM64 `libminecraftpe.so` SHA256：`9644b5d4eba3862cfe51ad760da5dbc8d85cfd0423884a048ab2295da64040b8`。
- 库文件大小：323630848 字节。
- 使用 manifest badging、ELF 程序段、RTTI/vtable 引用及 ARM64 序列化函数/字段 schema 检查格式。这是静态证据，没有在本机执行 Android 游戏库。
- 本地汇总：`runtime/netease310/latest/VERIFIED.md`；APK、库和完整汇编保留在忽略目录，不打入源码或发布包。
- REA 的最新 ELF 身份记录：`ev_fa5d3094d224da4ca40aa753ed2bcd39c0526ccaa94d21d6b9f3fa9b27bfa524`。该记录仅确认 ELF 身份；字段顺序依据另存的汇编/schema 分析，不由文件头推断。

## 协议变化

| 流程 | 当前处理及最新 APK 证据 |
| --- | --- |
| StartGame | 标准尾为 checksum uint64、UUID、三个 bool，共 **27 字节**；删除旧 TickDeathSystemsEnabled。最新写函数 `0xab7642c`，三个 bool 后 `0xab76920` 开始网易扩展。保留哈希方块 ID，校验和清零并关闭客户端地形生成。 |
| ResourcePackStack | 删除标准 BehaviourPacks 列表；保留纹理列表、引擎版本、实验设置和两个网易附加列表。最新写函数 `0xad126d0`。 |
| Text | 使用 898 的类别、字符串类型标签和可选过滤消息。chat/whisper/announcement 的 AuthorAndMessage 都有附加 Parameter List，位于消息主体内、XUID 之前（注册点 `0xab92110`、外层 XUID `0xab93180`）。translate/popup/jukeboxPopup 只有普通 Message 与 Parameter List，不再追加旧 Popup 包尾字符串。 |
| CommandRequest | 网易字段顺序为 Command → Origin → Internal → **UnLimit → Version**。`UnLimit` 注册点 `0xa0746c8`；桥接剥离中间 bool，保留版本字符串，不能删除包尾。 |
| 命令身份及回执 | OriginType/OutputType/Version 使用字符串枚举；PlayerId 固定 int64，SuccessCount 固定 uint32。UUID 映射跳过完整来源字符串。CommandOutput 没有额外网易包尾。 |
| 命令版本 | `Latest` 是有效枚举名；旧整数的字符串 `"42"` 会被 BDS 拒绝为 BadPacket。最新注册点 `0xa835d98`，值为 45；集成测试使用 `Latest`。 |
| PlayerAuthInput | 最新写函数仍使用 67 位原生输入标志，Delta 后 bool 与原生尾部保留；转换后使用 898 标准包。 |
| Animate | 两端 Action 均为 uint8 编号 `0/1/3/4/5`，公共字段为 Varuint64 runtime ID、无条件 Float32 Data 及可选小写字符串 SwingSource。网易为所有动作附加 zigzag Varint64 `mAttackId`：上行剥离，下行补原生默认 `-1`。原生注册 `0xa2a43f0` 为 Action 写入 metadata `4`，选择 numeric enum 分支；不能仅由字符串枚举注册推断网络编码。实机样本已确认。 |

未改变布局的公共包继续使用标准透传或原有网易扩展转换。历史 860 权限/世界扩展 fixture 作为原始证据保留；它们不是最新客户端的实机抓包。

## 配置与资源迁移

- 默认房间版本、示例配置及资源导入登录/握手更新至 1.21.130 / 898。新版网页沿用主分支的侧栏和多服务器管理，内核版本由配置管理。
- 配置加载时将顶层与各账号房间设置中的旧默认 `1.21.120.0` 迁移为 `1.21.130.0` 并保存，其他手动版本不被覆盖。
- 连接后端前检查 Pong 的游戏协议；旧 860 后端会得到版本不兼容提示。本版不实现两个标准游戏协议之间的转换。
- 内置经典材质元数据删除旧的空 BehaviourPacks 列表后使用 898 编码；组件编号、包身份及两个网易附加列表保留。
- 外部 `game_protocol=860` 资源配置只在旧行为包列表为空、新格式均校验通过时在内存迁移，原私有文件保留。非空行为包或损坏配置需要从网易 3.10 原生房间重新导入。
- 下载及启动脚本使用精确官方 BDS `1.21.130.4`，并明确启用 `block-network-ids-are-hashes=true`。
- 合并主分支 Geyser 转换后，修正哈希模式标志为 898 包尾倒数第 2 字节。独立取得 Geyser `b728af2e89d77d4d0f55bbc90af10ba612b70a03` 的 `block_palette.1_21_130.nbt`，确认与历史 860 表字节完全相同；15,845 个状态与原有索引/哈希向量保持一致。表可共用，但后端仍须协商协议 898；这不等于已完成 Geyser 898 实机验收。

## 正式发行范围

v1.3.0 以 GitHub 主分支 `220313d` 为基础，包含新版控制台、多服务器、安全更新和皮肤登录压缩修复。此次发行移植网易 3.10 适配与实机修复；本地投影和服务端 API 实验保留在原实验分支，未纳入正式版。

## FGAuth 配套修改

官方 pack list 已全量公布 `3.10.5.301327`。FGAuth 原有 3.10.0 的精确参数无法覆盖新版；直接选到新版会落入旧摘要/签名参数，因此也进行了源码修正：

- 默认认证引擎、MPay 应用版本及 versionCode 更新为最新 APK。
- 新版本库 MD5 为 `393dfdf6f78831f80a7e6aa305b21227`；签名证书 MD5 保持 `2b3e7ca013bb30a74d822579860c042b`。
- 原生签名调用 `0xe709070` / `0xe709074` 明确加载参数 `2,8`，`0xe709078` 调用签名函数 `0xe91c2dc`。
- 为确切新版绑定参数，保留 3.10.0 和 3.9；补丁版本和资源摘要继续读取官方元数据。
- FGAuth 根项目与嵌套 g79client 测试、服务构建通过。详细记录在相邻 FGAuth 项目的 `docs/NETEASE_3_10.md`。
- 后续按用户要求启动了更新后的本地 FGAuth 与桥接，已用原有账号创建房间并完成实际客户端进服；原有用户数据与密钥保留。

## 验证

### 源码检查

- `go test ./cmd/... ./internal/...`。
- `go vet ./cmd/... ./internal/...`。
- 构建 `cmd/nemcbridge`。
- 回归覆盖配置持久迁移、后端版本拒绝、资源配置迁移/拒绝、固定聊天字节样本、命令字段顺序与 UUID 映射、原生资源扩展和 27 字节世界尾部。

本地 `runtime/` 中有历史诊断 Go 程序，部分依赖旧类型，另有需通过 overlay 挂载的 benchmark；直接 `go test ./...` 会将这些忽略的临时程序也纳入。上述检查涵盖实际应用及全部 `internal` 包；发行 CI 的干净源码中没有该忽略目录。

### 官方 BDS 实测

官方 Linux ZIP SHA256：`96f707749e0c98d7c861c5ef740f8deaa3de5db65e166db41ba221cf76b4fd01`，75241415 字节，ZIP CRC 校验通过。

在独立 Ubuntu amd64 容器、独立世界及本机回环 UDP 19160–19163 上验证：

- 898 网络设置、离线加密登录、普通/缓存区块、完整 WebRTC 世界交换。
- 双向 Unicode 聊天、网易 XUID 身份、64/128/256 像素皮肤兼容。
- BDS 与 WebRTC 连续命令，包括错误命令后继续执行成功命令。
- Animate 修复：固定字节样本、五种动作、全部可选 SwingSource、跨多字节 runtime ID、负攻击者 ID、截断/未知字段拒绝、分客户端头部及压缩批次相邻包保留。BDS 与 WebRTC 分别发送三轮共 96 次原生挥手，每轮后成功执行命令。
- 两种传输的资源跳过、显式下载、提前响应后的自动下载及内置经典材质合并。
- 双资源包：619 字节/1 片及 1050840 字节/11 片，校验 ZIP 内容与 SHA256，并完成 Stack/StartGame。

测试中发现并修复了命令字段顺序与版本枚举问题；最初合并测试的重复 UID 冲突通过拆分命令测试身份修复。最终在同一全新 BDS 进程连续运行普通世界、皮肤、聊天及命令组合，10 项顶层测试及 6 项皮肤子测试全部通过，耗时 17.535 秒。原始失败及最终通过的日志均保留在 `runtime/netease310/`，最终组合日志为 `plain-final-integration.log`，汇总为 `validation-summary.md` 和 `bds-validation.json`。测试容器在完成后停止，没有修改生产世界。

首次 Animate 分析遗漏 Action descriptor 的 numeric enum metadata，误用字符串动作重建测试包；当时的 `animate-before-fix.log` / `animate-after-fix.log` 只证明错误构造的合成场景，不能证明实机兼容。用户复测仍失败，桥接于 21:55:13（UTC+8）抓到真实帧 `2c01c0010000000001046d696e6501`：ID44、action1、runtime192、Data0、source=`mine`、attack=-1。这份样本加入固定回归，第二版保留 byte action，仅处理攻击者扩展。新的正确编码集成日志为 `runtime/netease310/animate-numeric-after-fix.log`；下行由独立 codec 校验，真实客户端多人动画仍待验收。

第二版 `v1.3.0+netease310.animatefix2` 上线后，用户在房间 `399626` 复测连续挥手、挖掘并回复“正常了”。这确认了本次实机掉线修复；其他资源渲染、多人动画及商城/模组语义不据此视为全部验收。

## 正式源码复测

移植到未包含实验模块的正式源码后，再次通过 `go test ./...`、`go vet ./...` 和构建；官方 BDS 世界/皮肤/聊天/命令/挥手组合测试通过（25.266 秒），资源下载组合通过（17.205 秒）。原始日志保存在本地诊断目录，未包含于发布包。

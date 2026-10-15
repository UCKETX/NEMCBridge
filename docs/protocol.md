# 协议适配说明

本文说明当前转换逻辑及其证据边界。适配目标是网易房间协议 42、游戏协议 860；已验证版本见 [兼容性说明](compatibility.md)。

## 来源与归属

- `nemc-tan-lobby-solver` v0.2.8：房间、RakNet、NetherNet、WebRTC 等基础实现，本地修改位于 `vendor-src/`。
- `gophertunnel` v1.51.1：标准 Bedrock 协议结构、加密及数据编码。
- Eulogist 4.8.0：对标准与网易包布局进行静态比较，作为字段顺序和默认编码的参考；本仓库不包含其可执行文件或应用源码。
- `UCKETX/neomega-admin`：皮肤 RGBA、geometry、resource patch 的处理参考；它不提供游戏客户端之间的完整皮肤转换。
- neomega-core 的玩家资料字段及 ConfirmSkin 包命名，用于交叉核对网易扩展。

静态观察或字段推断不等同于完整游戏语义验证。当前可确认的真实客户端结果单独记录在兼容性文档中。

## 主要差异

| 包或流程 | 当前处理 |
| --- | --- |
| StartGame | 保留哈希方块网络 ID，整张方块表校验和置零，关闭客户端地形生成，并追加网易世界设置字段 |
| ResourcePacksInfo / ResourcePackStack | 网易方向清空资源包与行为包、取消强制要求，并补齐 Stack 两个空数组 |
| BiomeDefinitionList | 每条定义的公共固定字段后补 int32 与 string，保留标签与字典，移除客户端地形生成参数 |
| PlayerList / AddPlayer | 补网易资料及玩家生成扩展，保留皮肤和对应身份信息 |
| PlayerSkin / ConfirmSkin | 处理纹理和模型兼容；网易列表添加或换肤后追加皮肤确认包（ID 228） |
| UpdateAbilities | 能力层计数由 uint8 转 Varuint32，追加网易字段 |
| CraftingData / SetPlayerInventoryOptions | 补默认扩展；反向删除网易专有字段 |
| Text | Chat 追加字符串数组，Popup 追加字符串；反向读取并移除完整扩展 |
| PlayerAuthInput | 转换输入位标记，移除网易额外字段，保留公共交互与物品请求 |
| 容器编号 | 标准 0–16 保持，17–63 对应网易 18–64；处理内容、槽位、请求和响应 |

### 世界初始化 fixture

[`netease-startgame-extensions-zero.bin`](https://github.com/UCKETX/NEMCBridge/blob/main/internal/netease/testdata/netease-startgame-extensions-zero.bin) 及同目录 JSON 用于验证 32 个网易扩展字段的顺序和 50 字节零值编码。

参考程序 SHA256：`f8d4fe30f560b49d98a2e0522d9c94fed316eb68e8ed14f2524058e08530dfee`。标准 StartGame Marshal 参考地址为 `0x140587480` / `0x141431340`，网易 Marshal 为 `0x140e4ec80`；fixture 对应 `0x140e4fb26` 至 `0x140e4ff02` 的默认字段序列。

fixture 来自按已恢复 IO 方法宽度执行的受控静态模拟，不包含真实玩家数据，不表示执行了完整原程序或完整 StartGame 负载。它只能验证所覆盖的扩展格式。

### 皮肤

BDS 1.21.124 会丢弃测试中的 256×256 登录皮肤；实现将高清纹理适配到 128，处理旧模型描述、纹理尺寸和 UV，并在向后端登录时清除外部商城身份字段。网易方向的 ConfirmSkin 补发解决了已验证组合中的他人皮肤显示。

ConfirmSkin 的网易语义并未被完整还原，当前编码基于包结构对照和真实客户端结果；不能由此推断任意自定义或商城皮肤都受支持。

### 批次与传输

解压后的游戏批次上限为 32 MiB，压缩及未压缩路径均检查边界。NetherNet 分片内部计数使用 int；线上继续使用 1/0 延续标志，避免 uint8 计数超过 255 片时错误结束消息。

## 验证方式与未覆盖范围

协议格式测试、异常输入测试及真实 BDS/WebRTC 测试互为补充。自动测试覆盖的登录、加密、资源包交换、初始化及地形传输，不能代替客户端实际渲染和交互验证。

所有命令枚举、实体 metadata、特殊物品、模组语义及未来版本的兼容尚未全面验证。新增字段时应注明依据、保留未知边界并补充针对性测试，见 [参与开发](../CONTRIBUTING.md)。

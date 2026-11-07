# NEMCBridge 适配状态清单

2026-11-07 补充：[其他基岩服务端兼容性验证](server-compatibility.md)记录各实现与历史协议的独立结果。下列既有展示世界/手机反馈不代表所有服务端都已通过。

核对日期：2026-11-04。工作区版本 .128，资源包 0.8.53。以当前代码、docs/multiversion.md 和本次聊天实测反馈为准；不是正式发布说明。

本轮范围限定为 26.60 实际提供的内容；注册表中预留的名称不代表功能已经开放。唱片压制机 UI 不在本轮范围内。2026-11-04 用户要求冰系暂不适配：冰晶、冰锥、霜咬怪及冰弹暂停，不开启相关实验。

“已验证”仅覆盖表中写明的测试路径；“部分”包含替代外观、近似行为或未完成的特殊功能。用户要求暂放不等于问题已修复。

| 类别 | 项目 | 状态 | 范围与剩余问题 |
|---|---|---|---|
| 协议 | 1.21.124、1.21.130、26.0–26.60 所列协议 | 已接入 | 具体 BDS 构建自动验证；不能保证每个补丁版每项玩法均实测 |
| 协议 | 登录、聊天、命令、区块 | 已验证主要路径 | 26.60 地形可加载；/time 命令已修正 |
| 协议 | 背包、容器、物品转移 | 已验证主要路径 | 开背包卡死、移物断线已修复 |
| 容器 | 漏斗、投掷器、箱子流转 | 代表项已验证 | 白杨木板、朱砂、坐垫及含内芯硫方怪桶：转移、丢出拾取、数量与内芯保留手机反馈正常 |
| 容器 | 发射器使用新增物品 | 代表项已验证 | 硫方怪刷怪蛋、白杨船、含内芯硫方怪桶与原版对照：用户确认生成/放出、扣量及内芯保留正常 |
| 容器 | 潜影盒嵌套存储 | 用户反馈正常 | 用户确认本轮测试正常；材料发放曾因离线失败，实际测试物品及逐项结果未单独记录，不扩展为所有嵌套 NBT 已验证 |
| 半砖 | 新增半砖叠合预测 | 代表项已验证 | .123 六面假方块纠正、相邻放置、上下叠合、扣料和掉落获手机确认；测试材料为白杨、朱砂、红色混凝土/羊毛，橡木作对照 |
| 实体交互 | 悦灵手持与收集 | 代表项已验证 | 白杨木板、金蒲公英与橡木木板对照：手持显示、收集交回及取回种类/数量获用户确认 |
| 方块交互 | 着火与燃烧同步 | 代表项已验证 | 白杨木板、红色羊毛半砖/楼梯与原版对照：用户反馈本轮火焰位置及熄灭/烧毁残留检查通过；未据此审计全部可燃性参数 |
| 方块交互 | 支撑拆除与脱落 | 代表项已验证 | 白杨门、按钮、贴墙/悬挂告示牌、树苗及金蒲公英：用户确认脱落/掉落、残留模型和碰撞及重进检查正常 |
| 维度 | 主世界/下界往返 | 代表项已验证 | 新增物品、下界白杨楼梯/朱砂墙/文字告示牌以及带回主世界的硫方怪桶内芯，用户反馈正常 |
| 花盆 | 白杨树苗、金蒲公英 | 已验证 | .120 盆内显示、.122 顶面/侧面假花纠正，以及取出/打破掉落获手机确认；红色灌木原生不支持入盆 |
| 展示框 | 普通/荧光框的新增物品 | 代表项已验证 | .118 修复透明；白杨楼梯、Bounce、硫方怪桶的显示/旋转/重进及取出内芯保留手机反馈正常 |
| 铁砧 | 新增物品重命名 | 代表项已验证 | 白杨木板、金蒲公英及含内芯硫方怪桶：取出、经验、数量、名称及内芯保留手机反馈正常 |
| 身份 | 玩家身份、存档、OP 重进保留 | 已验证 | 网易 XUID/PFID 仍非真实 Xbox 身份；通过桥接身份/权限持久化处理 |
| 资源 | 桥接按版本下载、校验、缓存并自动下发 | 已验证 | 服主无需放包；国际版直接连接不会收到桥接注入资源 |
| 移动 | 斜向视角直走抽动 | 暂缓 | 某些区域仍出现，未最终解决 |
| 方块 | 硫磺、朱砂完整建筑系列 | 已适配、代表项已验证 | 基础/砖/磨制/雕纹及楼梯、半砖、墙；含水问题另列 |
| 方块 | 16 色混凝土楼梯、半砖/双半砖 | 已适配、代表项已验证 | 外观、手持、方向和碰撞；未逐颜色逐玩法验证 |
| 方块 | 16 色羊毛楼梯、半砖/双半砖 | 已适配、代表项已验证 | 独立外观；不可燃符合原生审计 |
| 方块 | 新增楼梯普通、倒置、转角、手持方向 | 已验证 | 已修正形状、方向及整块碰撞 |
| 方块 | 新增墙连接和防跳碰撞 | 已验证 | 硫磺/朱砂墙已通过测试 |
| 白杨 | 原木、木头、去皮变体、木板 | 已验证 | 外观、去皮方向、声音、耐久和掉落 |
| 白杨 | 三色树叶、树苗 | 已验证主要路径 | 生长、外观、镂空和交界闪烁已处理 |
| 白杨 | 木门、活板门 | 已验证 | 贴图/模型、方向、开关、碰撞、手持交互假方块修复 |
| 白杨 | 按钮、压力板 | 已验证 | 外观、按压/复位、红石；按钮图标比例/方向及误放置已修正 |
| 白杨 | 栅栏、栅栏门 | 已验证 | 连接、通行、防跳、红石及物品模型；原版栅栏误放置修复 |
| 白杨 | 普通/贴墙/悬挂告示牌 | 已验证 | 独立牌体、四向位置、文字编辑、染色及荧光；普通/悬挂告示牌双面封蜡、反馈、扣量及重进保留获用户确认 |
| 白杨 | 置物架 | 已验证 | 物品/手持/放置、四向、三槽存取、红石及拆除 |
| 植物 | 红色灌木 | 已验证 | 外观、骨粉扩散及堆肥 |
| 植物 | 层架蘑菇外观、生长、碰撞、堆肥 | 已验证 | 大小和四向碰撞已修正 |
| 植物 | 层架蘑菇弹跳 | 部分，暂缓 | .109 单次客户端补偿；非完整原生连续衰减，用户要求停止继续调整 |
| 植物 | 金蒲公英外观与暂停/恢复幼年生长 | 已验证主要路径 | 独立物品/地面外观、交互扣料、提示和声音；粒子可见性采用近似外围布局 |
| 植物 | 金蒲公英堆肥 | 原生不支持 | 原生 BDS 对照同样不可堆肥，不当作兼容缺陷 |
| 方块 | 硫磺尖刺 | 已验证主要路径 | 10 状态、外观和碰撞；位置扰动及专属音效未完整复刻 |
| 实体 | 硫方怪、刷怪蛋、桶 | 已适配、主要路径已验证 | 吞入/剪出、内芯、攻击方向和掉线修复；装桶/放出、扣桶及内芯保留获手机确认；.114 桶内芯名称提示、装桶/放出及内芯保留已获手机确认 |
| 实体 | 硫方怪吞入 TNT、原木、矿石及新增材料 | 已验证代表项 | 内芯已扩展，尺寸 15 像素；外壳透明度非完全原生；手机确认.125引信声/爆炸正常；0.8.54闪白导致所有内芯透明，.126撤回后内芯及声音已确认正常；.127资源表达式仍导致全内芯透明；.128按用户要求恢复0.8.53，仅保留声音；闪烁明确暂停 |
| 实体 | 新版幼年动物模型 | 已适配、代表项已验证 | 展示含 40 类对照；不声称每个物种每个动画均手机验收 |
| 实体 | 16 色坐垫 | 已验证主要路径 | 外观、朝向、坐下/起身、提示、阴影；用户已确认白色/红色坐垫创造取物、整组16个及放置坐下正常 |
| 实体 | 白杨船、运输船 | 部分，采用替代 | 物品外观独立，放置后用白桦船体；驾驶、储物正常；按用户要求停止独立动画方案 |
| 方块 | 稻草床 | 部分，暂缓 | 能睡眠、跳夜、消耗；不实际设重生点；临时红床闪现仍有局限 |
| 音乐 | Bounce 唱片 | 已验证 | 图标/名称、原曲、插入取出、破坏唱片机停播；旧唱片停播也已修复 |
| 物品 | 相片簿 portfolio | 部分 | 静态物品外观已验证；Education 相片簿页面未实现 |
| 配方 | 合成、混合木材、染色、切石、木炭烧制 | 已验证代表路径 | 当前 BDS 4770 条配方均下发；不等于逐条实测 |
| 燃料 | 全部独立物品燃料属性 | 已审计、代表项已验证 | 178 项原生审计，36 可燃/142 不燃；不伪造燃料能力 |
| 挖掘 | 空手挖硫磺/朱砂/混凝土与白杨系列 | 已验证 | 已补挖掘耗时；所有工具/附魔/状态组合精确一致尚未证明 |
| 掉落 | 白杨树叶、硫磺/朱砂普通/精准采集及剪刀 | 用户反馈正常 | 未逐个保存所有组合的服务端测量结果 |
| 活塞 | 新增方块推动/拉回外观 | 已验证实时路径 | .110 实时路径已验证；.117 补子区块内嵌移动方块 NBT，自动测试通过；手机展示区/推拉/重进回归正常，未专门捕获移动瞬间加载；完整列尾部尚未覆盖 |
| 含水 | 楼梯含水 | 部分 | 早期反馈正常；最新反馈指出新增含水方块普遍有放水假水，需随 .116 复验 |
| 含水 | 新增半砖 | 已验证主要路径 | .116 修正水桶注册表识别后，手机反馈放水假水消失，含水/取水正常 |
| 含水 | 新增墙及其他独立含水方块 | 代表项已验证 | .116 通用刷新实际触发；朱砂墙及半砖放水/取水手机反馈正常，未逐材质验证 |
| 语言 | 新增名称、按钮及提示 | 已适配主要范围 | 中英文各补 379 键；代表文本已收到，不代表所有键/参数已逐条确认 |
| 范围外 | 唱片压制机 disc_press | 本轮不适配 | 26.60 注册表存在预留名，但 Music Features Add-On 的唱片压制机功能在 26.70.21 预览开放；保留既有回退，不加入新 UI |
| 暂缓 | 冰晶 ice_crystal、冰锥 icicle | 用户要求暂停 | 回退紫水晶簇/滴水石锥；26.60 的 Drop 4 of 2026 实验内容，当前展示世界未开启实验，暂停启用与独立外观适配 |
| 暂缓 | 霜咬怪 frostbite、刷怪蛋、冰弹 ice_ball | 用户要求暂停 | 目前流浪者/流浪者蛋/雪球回退；26.60 的 Drop 4 of 2026 实验内容，当前展示世界未开启实验；独立模型与效果待适配 |
| 未完成 | 照片 photo_item | 未适配 | 普通纸回退；动态照片内容未实现 |
| 范围外 | 任意模组、自定义行为包/客户端脚本、新工作站 UI | 无通用保证 | 不可从原版内容映射推断兼容 |
| Geyser 顺序编号 | 全部已列 859–2225 协议档位 | 合成客户端实服验证通过 | 原版表双向转换；关闭 Geyser 自定义内容，详见 multiversion.md 的 Geyser 验证 |
| 范围外 | 未列协议、自定义顺序方块表 | 未支持 | BDS 保持哈希方块编号；不接受未知表或自定义定义 |

版本范围依据：[26.60 更新说明](https://feedback.minecraft.net/hc/en-us/articles/49852828563853-Minecraft-Bedrock-Edition-26-60-Changelog)、[26.70.21 预览说明](https://feedback.minecraft.net/hc/en-us/articles/49758922088845-Minecraft-Beta-Preview-26-70-21)。

## 静态覆盖统计

本次重新运行 audit_content_coverage.py：185 新增物品、159 新增方块名、8684 新增状态，151 独立世界定义，未映射错误 0。177 个直接物品定义加金蒲公英共 178 项。7 个剩余替代注册名中的 poplar_standing_sign/poplar_wall_sign 已有独立覆盖层，不是视觉缺失；其余 5 个为 disc_press、frostbite_spawn_egg、ice_crystal、icicle、photo_item。船体等部分适配不体现在这个数字中。

## 全部 185 个新增物品注册名

以下仅表示代码实现路径；逐项玩法验收以主表为准。双半砖、放置态名称也可能属于注册表条目，不能将条目数直接当作玩法数。

| 注册名 | 实现路径 |
|---|---|
| `minecraft:black_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:black_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:black_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:black_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:black_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:black_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:black_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:blue_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:brown_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:chiseled_cinnabar` | 独立物品定义（特殊限制见主表） |
| `minecraft:chiseled_sulfur` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_brick_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_brick_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_brick_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_brick_wall` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_bricks` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:cinnabar_wall` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:cyan_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:disc_press` | 替代为 minecraft:crafting_table |
| `minecraft:frostbite_spawn_egg` | 替代为 minecraft:stray_spawn_egg |
| `minecraft:golden_dandelion` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:gray_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:green_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:ice_crystal` | 替代为 minecraft:amethyst_cluster |
| `minecraft:icicle` | 替代为 minecraft:pointed_dripstone |
| `minecraft:item.straw_bed` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_blue_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:light_gray_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:lime_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:magenta_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:music_disc_bounce` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_poplar_leaves` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:orange_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:photo_item` | 替代为 minecraft:paper |
| `minecraft:pink_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:pink_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:pink_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:pink_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:pink_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:pink_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:pink_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_cinnabar` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_cinnabar_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_cinnabar_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_cinnabar_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_cinnabar_wall` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_sulfur` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_sulfur_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_sulfur_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_sulfur_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:polished_sulfur_wall` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_boat` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_button` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_chest_boat` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_door` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_fence` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_fence_gate` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_hanging_sign` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_log` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_planks` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_pressure_plate` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_sapling` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_shelf` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_sign` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_standing_sign` | 原生告示牌交互 + 独立白杨覆盖层 |
| `minecraft:poplar_trapdoor` | 独立物品定义（特殊限制见主表） |
| `minecraft:poplar_wall_sign` | 原生告示牌交互 + 独立白杨覆盖层 |
| `minecraft:poplar_wood` | 独立物品定义（特殊限制见主表） |
| `minecraft:portfolio` | 独立物品定义（特殊限制见主表） |
| `minecraft:potent_sulfur` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:purple_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_poplar_leaves` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_shrub` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:red_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:shelf_mushroom` | 独立物品定义（特殊限制见主表） |
| `minecraft:straw_bed` | 独立物品定义（特殊限制见主表） |
| `minecraft:stripped_poplar_log` | 独立物品定义（特殊限制见主表） |
| `minecraft:stripped_poplar_wood` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_brick_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_brick_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_brick_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_brick_wall` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_bricks` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_cube_bucket` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_cube_spawn_egg` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_spike` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:sulfur_wall` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:white_wool_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_concrete_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_concrete_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_concrete_stairs` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_cushion` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_poplar_leaves` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_wool_double_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_wool_slab` | 独立物品定义（特殊限制见主表） |
| `minecraft:yellow_wool_stairs` | 独立物品定义（特殊限制见主表） |

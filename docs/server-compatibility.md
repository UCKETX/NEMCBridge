# 基岩版服务端兼容范围

v1.4.1 支持所列 1.21.120–26.60 协议档位，具体版本见[多版本说明](multiversion.md)。国际版直连客户端仍需匹配后端。

## 已验证组合

- Geyser：859、860、898、924、944、975、1001、2168、2169、2193、2225，限匹配的原版方块表，关闭自定义内容。
- Allay 0.14.0、PocketMine-MP 5.44.3：协议 1001。
- PowerNukkitX 3.0.6、Nukkit snapshot 1252、Nukkit-MOT 1766、Cloudburst snapshot 162、Dragonfly 本次固定构建：协议 2225。
- Endstone 0.11.15 + BDS 26.60.6、LeviLamina 26.60.4 + Windows BDS 26.60.6：协议 2225。
- LeviLamina 历史已发行组合：1.21.124、1.21.132、26.10、26.20、26.32、26.40、26.51。

以上为固定构建的直连与 WebRTC 基础验证，涵盖进服、地形、聊天及最新构建的背包取物、槽位转移与重连保存；不代表所有插件、模组、玩法或真实手机客户端均已验证。

## 配置与限制

- Nukkit 2168+：在国际服设置中选择 **Nukkit 兼容配置**，或设置 `compatibility_profile: "nukkit"`。
- 后端需关闭 Xbox 登录验证；BDS 使用 `online-mode=false`、`compression-algorithm=zlib`、`block-network-ids-are-hashes=true`。其他实现按其配置方式设置，见[部署指南](setup.md)。
- LeviLamina 26.0、26.45 未找到匹配的官方已发行加载器/BDS 组合，未实服验证。
- 部分旧 Cloudburst 构建存在启动、聊天或注册表故障，建议使用已验证的新构建。旧 PowerNukkitX 2.0.0 官方产物实际声明协议 1001，不能作为 975 的支持依据。
- 不保证任意自定义方块、资源包、第三方插件与模组兼容。

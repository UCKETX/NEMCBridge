# NEMCBridge 发布版

用于网易我的世界“本地联机”持续开房，并将玩家连接转发到已有的 Bedrock 服务器。

已实测组合：网易 3.9.41.300665、房间版本 1.21.120.0/协议 42、BDS 1.21.124.2/游戏协议 860。Windows 实机确认进入世界、区块加载、方块破坏及放置正常；Linux/macOS 构建和自动测试由 Actions 验证，游戏实机互通仍以实际环境为准。

## 启动

1. 先运行自己的 Bedrock 服务器，默认目标为 `127.0.0.1:19132`。设置 `online-mode=false`、`compression-algorithm=zlib`（Flate 压缩）、`block-network-ids-are-hashes=true`。
2. Windows 双击 `NEMCBridge.exe`；Linux/macOS 在解压目录运行 `./NEMCBridge -data-dir ./data`。Linux/macOS 若权限未保留，先执行 `chmod +x NEMCBridge`。
3. 在浏览器打开 `http://127.0.0.1:18321/`，设置 FGAuth 验证服务器、自己的机器人令牌及目标服务器，保存后点击“开房”。
4. 按控制台显示的房间号加入网易“本地联机”。勾选“程序启动时自动开房”后，下次启动会自动开房；重连可能改变房间号。

Windows 的令牌使用当前用户 DPAPI 加密，Linux/macOS 以 0600 文件权限保存。不同平台不能直接复用 Windows 加密文件；在新环境重新配置自己的令牌。

同一机器人账号应只保持一个登录会话，重复登录可能使当前房间掉线。新版通过系统文件锁阻止本机同一用户使用相同令牌重复开房或鉴权检查；无法阻止旧版本、其他软件/设备或同账号使用不同令牌登录。

聊天和皮肤互显已实机确认正常，已修复原 2MB 批次限制及超过 255 片的传输错误。网易端会跳过服务端资源包与行为包，使用客户端现有资源；国际版直连 BDS 的资源包流程不受影响。依赖这些包的自定义内容仍需客户端支持。

## 命令行

```sh
./NEMCBridge -version
./NEMCBridge -headless -data-dir ./data -web-port 18321
```

`NEMCBRIDGE_TOKEN` 环境变量可提供令牌，也可以用 `-import-token` 从标准输入导入。控制台只监听本机；远程服务器可通过 SSH 转发访问控制台。Ctrl+C 或 SIGTERM 正常关闭房间和连接；网页也可停止房间。

`config.example.json` 仅包含默认配置。Release 不含凭据、服务器程序、世界或日志。Web 界面已嵌入二进制，无需单独安装。

下载后可用对应 `.sha256` 文件验证压缩包，例如 `sha256sum -c LobbyKeeper_v0.1.2_linux_amd64.tar.gz.sha256`。


## 更多说明与许可

部署与升级见 [部署指南](setup.md)，支持版本及已知限制见 [兼容性说明](compatibility.md)。后端离线身份不代表 Xbox 认证，远程访问需配置适当的网络和游戏权限。

原创代码与文档采用 [MIT License](../LICENSE)。第三方组件保留原有许可；部分上游模块的整体许可尚待确认，详情见 [第三方声明](../THIRD_PARTY_NOTICES.md)。本项目不代表 Mojang、Microsoft 或网易。

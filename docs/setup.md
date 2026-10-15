# 部署指南

## 前提

- 能运行已适配版本的 Bedrock Dedicated Server；当前实测为 1.21.124.2。
- FGAuth 验证服务器及属于自己的房主机器人令牌。令牌由所选服务提供，本项目不附带账号或令牌。
- 挂服机可连接验证服务器、网易房间节点和后端 UDP 端口；玩家侧需要可用的 WebRTC 网络连接。

FGAuth 默认地址是 `https://fgauth.ucket.top`，可在控制台改为兼容服务。房主账号通过所选服务鉴权，令牌会发送到该服务，请使用自己信任的服务。

## 后端设置

在 BDS 的 `server.properties` 中设置：

```properties
online-mode=false
compression-algorithm=zlib
block-network-ids-are-hashes=true
```

`zlib` 对应当前桥接支持的 Flate 压缩。房间版本字段和 BDS 版本是两个独立配置；已验证房间版本为 `1.21.120.0`，不要按后端版本随意替换。

国际版玩家直接连接 BDS；需要远程直连时自行配置 UDP 端口转发、防火墙和访问权限。后端关闭 Xbox 登录认证后，名称、UUID、XUID 不能作为已认证账号的凭证。挂服机并不提供额外的玩家账号授权系统。

## 配置与数据

程序首次运行会在数据目录生成配置。Windows 双击程序时默认使用可执行文件所在目录；可用 `-data-dir` 指定独立目录。示例见 [`config.example.json`](../config.example.json)。

| 配置项 | 用途 |
| --- | --- |
| `auth_server` | FGAuth HTTPS 验证服务器；本机测试允许 HTTP |
| `upstream` | BDS 地址和 UDP 端口，默认 `127.0.0.1:19132` |
| `room_name` / `room_password` | 房间名称与密码 |
| `room_game_version` | 网易房间内核版本，当前适配 `1.21.120.0` |
| `max_players` | 房间人数，2–40 |
| `friends_only` / `allow_pvp` | 好友加入与 PvP 设置 |
| `auto_start` | 程序启动时自动开房 |
| `web_port` | 本机 Web 控制台端口，默认 18321 |
| `retry_seconds` | 掉线重试间隔，5–300 秒 |

令牌独立保存为 `secrets.dat`，不会写入 `config.json`。Windows 使用当前用户 DPAPI 加密；Linux/macOS 使用文件权限保护，不提供文件内容加密。跨系统迁移时请在新环境重新填写令牌。

运行日志位于数据目录的 `logs/keeper.log`，状态快照为 `logs/status.json`。分享日志前请检查账号、玩家信息、地址及其他个人内容。

## 命令行与远程管理

```sh
./NEMCBridge -version
./NEMCBridge -headless -data-dir ./data -web-port 18321
./NEMCBridge -headless -data-dir ./data -run
```

`-headless` 关闭自动打开浏览器；`-run` 在启动后自动开房。也可以通过 `NEMCBRIDGE_TOKEN` 环境变量提供令牌（兼容旧的 `LOBBYKEEPER_TOKEN`，新变量优先），或用 `-import-token` 从标准输入导入。完整参数用 `-help` 查看。

控制台只监听 `127.0.0.1`，远程服务器可使用 SSH 转发：

```sh
ssh -N -L 18321:127.0.0.1:18321 user@your-server
```

然后在本机打开 [http://127.0.0.1:18321/](http://127.0.0.1:18321/)。网页没有独立的登录认证，请保留本机监听，并通过受保护的通道访问。

Ctrl+C、SIGTERM 或网页的退出功能会正常关闭房间。停止房间不会关闭独立运行的 BDS。

## 升级与校验

1. 下载新版本及同名 `.sha256` 文件。
2. 校验压缩包，例如 Linux/macOS：`sha256sum -c NEMCBridge_v0.1.0_linux_amd64.tar.gz.sha256`。macOS 也可使用 `shasum -a 256 -c`；Windows 使用 `Get-FileHash` 与校验文件比对。
3. 正常退出旧挂服机，再替换可执行文件；保留自己的数据目录和 BDS 世界。
4. 启动后确认后端状态和新房间号。

## 常见问题

### 提示客户端与房主版本不同

先核对 [兼容版本表](compatibility.md)。网易客户端版本、房间版本及后端游戏协议需要分别匹配；修改服务器 MOTD 无法代替协议适配。

### 房间反复断开

确认同一机器人账号没有被其他程序、设备或客户端重复登录。本机相同用户、相同令牌有会话锁，但不能阻止其他设备或同账号的不同令牌。检查日志中的信令、鉴权及网络错误；自动重连不保证保留原房间号。

### 可以加入房间，但进不了世界

检查 BDS 地址、端口、版本、`online-mode`、压缩和方块 ID 设置。将已脱敏的连接阶段日志及双方完整版本号附在问题报告中。

### 看不到皮肤或资源包内容

国际版尝试关闭「仅允许受信任的皮肤」并让双方重新加入。特殊商城、4D 或其他自定义模型不保证兼容。网易端目前跳过后端资源包，不能依靠服务器下发包来显示自定义内容。

# 部署指南

## 前提

- 能运行已适配版本的 Bedrock Dedicated Server；当前实测为 1.21.124.2。
- FGAuth 验证服务器及属于自己的房主机器人令牌。令牌由所选服务提供，本项目不附带账号或令牌。
- 挂服机可连接验证服务器、网易房间节点和后端 UDP 端口；玩家侧需要可用的 WebRTC 网络连接。

FGAuth 默认地址是 `https://fgauth.ucket.top`，可在控制台改为其他兼容服务，不限制服务域名或 HTTPS 端口。程序会跟随验证站的 HTTPS 跨域跳转，支持 IPv4 / IPv6 分流。房主账号通过所选服务鉴权，令牌会发送到该服务，请使用自己信任的服务。

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
| `web_port` | Web 控制台端口，默认 18321，监听所有 IPv4 网卡 |
| `retry_seconds` | 掉线重试间隔，5–300 秒 |

令牌独立保存为 `secrets.dat`，不会写入 `config.json`。Windows 使用当前用户 DPAPI 加密；Linux/macOS 使用文件权限保护，不提供文件内容加密。跨系统迁移时请在新环境重新填写令牌。

运行日志位于数据目录的 `logs/keeper.log`，状态快照为 `logs/status.json`。分享日志前请检查账号、玩家信息、地址及其他个人内容。

## 管理员登录

打开页面后先登录。固定账号为 `admin`；首次启动会生成随机密码，只在终端显示一次，请妥善保存。密码以 bcrypt 哈希形式保存在数据目录的 `admin.json`，不写入配置或日志。管理密码与 FGAuth 机器人令牌是两项独立凭据。

也可以通过 `NEMCBRIDGE_ADMIN_PASSWORD` 环境变量设置自己的密码（8–72 字节）；环境变量中的密码不会回显。启动时该变量优先于已保存的密码，取消变量后仍可使用最后保存的密码。

忘记密码时，先正常退出程序，再保留原数据目录执行：

```sh
./NEMCBridge -data-dir ./data -reset-admin-password
```

Windows 对应 `NEMCBridge.exe -data-dir ./data -reset-admin-password`。重置会生成新密码并在终端显示；如设置了上述环境变量则使用该密码。

登录会话有效期为 12 小时，程序重启后需重新登录。点击「退出登录」会注销当前浏览器会话，不会停止房间。未登录时不能读取状态或调用管理接口；登录请求按客户端 IP 限速。

## 命令行与远程管理

```sh
./NEMCBridge -version
./NEMCBridge -headless -data-dir ./data -web-port 18321
./NEMCBridge -headless -data-dir ./data -run
```

`-headless` 关闭自动打开浏览器；`-run` 在启动后自动开房。也可以通过 `NEMCBRIDGE_TOKEN` 环境变量提供令牌（兼容旧的 `LOBBYKEEPER_TOKEN`，新变量优先），或用 `-import-token` 从标准输入导入。完整参数用 `-help` 查看。

控制台默认监听 `0.0.0.0`（所有 IPv4 网卡），终端会输出本机地址和各可用网卡的局域网访问地址。例如终端显示 `http://192.168.1.20:18321/` 时，同一网络的设备可使用该地址访问。`0.0.0.0` 是监听地址，访问时请使用机器的实际 IP。

控制台需要管理员登录。默认使用 HTTP，登录认证不替代传输加密；请通过防火墙将端口限制在可信网络内。远程服务器可使用 SSH 转发：

```sh
ssh -N -L 18321:127.0.0.1:18321 user@your-server
```

然后在本机打开 [http://127.0.0.1:18321/](http://127.0.0.1:18321/)。使用 SSH 转发时，仍应通过服务器防火墙限制管理端口的直接访问。

Ctrl+C、SIGTERM 或网页的退出功能会正常关闭房间。停止房间不会关闭独立运行的 BDS。

## 升级与校验

1. 下载新版本及同名 `.sha256` 文件。
2. 校验压缩包，例如 Linux/macOS：`sha256sum -c NEMCBridge_v0.1.2_linux_amd64.tar.gz.sha256`。macOS 也可使用 `shasum -a 256 -c`；Windows 使用 `Get-FileHash` 与校验文件比对。
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

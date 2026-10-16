<div align="center">

<img src="branding/logo/catflap.svg" width="120" alt="Catflap">

# Catflap · 猫洞

**没有公网 IP，也把家的门开给世界。**

内网穿透 / 反向代理 / DDNS / 证书 —— 一个 Go 单二进制，下载就能跑

[![License: GPL-3.0-or-later](https://img.shields.io/badge/License-GPL--3.0--or--later-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](go.mod)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-lightgrey.svg)](#安装)

</div>

---

## 这是什么

家里的 NAS、软路由、Jellyfin 想在外面打开，通常卡在三件事上：**运营商不给公网 IP**、
**家宽 IP 天天变**、**浏览器进去就是「不安全」**。

Catflap 把这三件事和它们的配套 —— 打洞、DDNS、证书、反向代理 —— 打包进同一个程序。
前端随 Go `embed` 一起打包，下载一个二进制就能跑，同时提供**导航主页**和**管理后台**两套界面。

名字取自**猫洞（cat flap）**：不必拆掉整扇门，只在门上开一个只让该进出的东西通过的小活门。
这正是它干的事 —— 不动路由器、不要公网 IP，在内网的墙上开一个洞。

## 代码来源与致谢

Catflap 是一个**独立项目**：有自己的名称、图标、功能路线和代码历史。
同时我们把「站在谁肩膀上」写清楚 —— 开发初期，内网穿透、DDNS、ACME 证书、
反向代理等模块是**以 [ZluxYao/LinkStar](https://github.com/ZluxYao/LinkStar)
0.7.4 的代码为起点**改写的，该项目以 **GPL-3.0-or-later** 发布，因此本项目
同样以 GPL-3.0-or-later 发布，并保留其全部版权与许可声明。

| 被引用的项目 | 许可 | 与本项目 |
|---|---|---|
| [LinkStar](https://github.com/ZluxYao/LinkStar)（ZluxYao） | GPL-3.0-or-later | **引用了代码**：0.7.4 版本为初始实现基线，见 [`NOTICE`](NOTICE) |
| [Lucky](https://github.com/gdy666/lucky)(gdy666) | MIT（≤v1.4.10） | **仅参照设计**：端口转发、计划任务等思路；未使用其任何代码 |
| gin / logrus / goupnp / pion-stun / wails / react / vite / tailwindcss 等 | MIT / BSD-3 / ISC | 直接依赖，完整清单见 `go.mod` 与 `web/*/package.json` |

三点说明：

- 本项目与 LinkStar 之间**没有** git 上的父子关系（本仓库是一份独立历史，
  不含上游提交）。上游后续更新我们采用**人工阅读、借鉴实现思路**的方式吸收，
  不做 `git merge`。
- 本项目**不提供**从 LinkStar 迁移的路径，两者是不同的产品。写进你 DNS
  服务商账号的标记也只认 `catflap:` 前缀。
- 本项目与上述项目之间**不存在**官方、赞助或背书关系；它们的插件、文档、
  社区资源不适用于本项目，也请不要把本项目的问题反馈到它们那里。

完整的改动清单见 [`NOTICE`](NOTICE) 与 [`CHANGELOG.md`](CHANGELOG.md)。

## 功能

| 能力 | 说明 |
|---|---|
| **内网穿透** | STUN 打洞直连 + UPnP；TCP / UDP |
| **NAT 类型检测** | 启动时探测本机 NAT 类型，提前告诉你「这台机器能不能被打通」 |
| **物理网卡出口** | 可指定走真实物理网卡出去，绕开 TUN / 代理虚拟网卡导致的打洞失败 |
| **TCP 保活加固** | 在 CGNAT 等不支持 NAT 回环的环境下，改为在同一条 STUN 连接上重新握手验证映射，并跟随新公网端口自动同步 |
| **STUN 服务器可配** | 面板内可编辑 STUN 服务器列表并立即重测选优（默认内置一批实测可用项） |
| **穿透固定端口** | 建服务时自动分配本机端口（也可自己指定），外部端口因此稳定；被占用自动退回随机 |
| **反向代理** | 站点规则 + TLS；一个入口端口承载全部二级域名（对外只放通一个）；站点级 IP 门禁 |
| **DDNS** | 公网 IP 变化后自动同步解析记录；**A / AAAA / 双栈**一条记录搞定；多家 DNS 服务商 |
| **证书** | 自签 / 上传 / ACME（含 DNS-01）自动签发与续期 |
| **固定入口** | Cloudflare 307 固定入口；Minecraft SRV 记录自动维护 |
| **Cloudflare Tunnel** | 托管 cloudflared，**URL 不带端口**、不用打洞与 DDNS、路由器不用放行（CGNAT 场景的最优解） |
| **通知中心** | 渠道（Webhook / 钉钉 / 企业微信 / 飞书 / Bark / Server 酱 / PushPlus）**全局配一次**，各服务按名字选用；可测试发送 |
| **IPv6 直连** | 有公网 IPv6 时零打洞直达；导航页一个图标覆盖 v4 / v6 / 内网三种入口 |
| **导航主页** | 分组卡片式门户；分组可设「要登录才可见」（服务端过滤，访客连组名都拿不到） |
| **服务索引** | 一个门户页列出所有已开放服务，`/go/{服务名}/` 直接跳转 |
| **流量统计** | 实时入/出站速率、累计流量、每日趋势、各服务流量 Top（仪表盘大屏） |
| **访问审计** | 谁在访问、在扫什么路径；按 IP 统计并支持一键拉黑与自动封禁 |
| **日志分类** | 按模块自动分类、折叠显示异常数量、点开看详情 |

## 目录结构

```
catflap/
├─ .github/                  协作配置：workflows / Issue 模板
├─ branding/                 ★ 品牌资产唯一来源（改品牌只动这里）
│  ├─ logo/  icon/           矢量图标、favicon、托盘小图
│  └─ README.md              色板、用法、导出规范
├─ core/                     框架层（Go 包）
│  ├─ component/  provider/  组件注册表、提供者注册表
│  ├─ bus/                   事件总线（模块间解耦）
│  └─ enter.go  logrus.go    日志、关闭信号等基础设施
├─ modules/                  业务模块（一个目录一个功能）
│  └─ auth/ cert/ ddns/ home/ notify/ proxy/ stun/ task/ tunnel/ update/ webhook/ wol/
├─ api/                      HTTP 接口层（按模块分子目录）
├─ routers/  middleware/     路由装配与中间件
├─ utils/                    无状态工具
├─ web/
│  ├─ admin/                 管理面板（React 19 + TS + Vite + Tailwind）
│  └─ home/                  导航主页
├─ deploy/                   ★ 部署物料：systemd / docker / 各平台步骤
├─ docs/                     文档，索引见 docs/README.md
│  ├─ guides/                ★ 按场景的部署指南（IPv6、单端口入口……）
│  ├─ decisions/             ★ 架构决策记录（ADR）
│  ├─ design/  compliance/   设计与路线图 / 许可与合规
│  └─ upstream/              归档：上游原始 README
├─ scripts/                  打包与校验脚本（fpk / 安装器 / 守门检查）
├─ build/                    桌面版打包资源（Wails）
├─ app.go  main_cli.go  main_desktop.go   后端启动与两个编译入口
├─ Taskfile.yml  BUILD.md    一键编译 / 编译说明
└─ AGENTS.md                 给 AI 编码助手的仓库约束
```

★ = 本项目新增/重点维护的部分。

### 关于目录的两条约定

- **Go 的包路径就是目录路径**：改 `modules/`、`api/` 这类目录等于全库改 import，
  动之前先想清楚；新增内容按功能归位（表见 CONTRIBUTING.md）。
- `docs/img/` 放 README 与文档配图；`config/`、`data/` 是运行期数据（含明文凭据），
  已 gitignore。

## 安装

### Debian / Ubuntu —— 一行命令（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/chenggegedeid/catflap/main/scripts/install.sh \
  -o /tmp/install.sh && sudo bash /tmp/install.sh
```

脚本会做的事：识别架构 → 拉最新 Release → **比对 sha256** → 建专用用户 `catflap`
（**不用 root 跑服务**）→ 装 systemd 单元 → 开机自启 → 打印访问地址。

**已经装过的话，它会先问你要干什么，不会闷头就开始下载**：

```
  检测到已安装 Catflap v2.0.0
    目录：/opt/catflap
    服务：运行中

  1 更新到最新版（保留配置与数据）
  2 重装 / 安装指定版本
  3 卸载（保留配置与数据）
  4 完全卸载（连配置与数据一起删，删前自动备份）
  0 退出
```

管道写法 `curl -fsSL .../install.sh | sudo bash` 也能用，一样会弹这个菜单。
推荐上面那种「先下载成文件」的写法，是为了让你手上有份脚本可以反复用
（`sudo bash /tmp/install.sh` 直接就是管理菜单）。

> 别把脚本的来源地址当参数传回来（`... install.sh "$URL"`）——
> Release 资产不在 raw.githubusercontent.com 上，传了会被当成镜像前缀拼进下载链接，
> 拼出 `.../main/https://github.com/...` 这种畸形 URL（curl 报 404）。

- 升级：再跑一次（默认就是第 1 项，只换二进制，配置与数据不动）
- 卸载：`sudo bash /tmp/install.sh --uninstall`（保留配置）；`--purge` 连配置一起删
  （**删之前会自动打包备份到 `/opt/catflap.bak-<时间戳>.tar.gz`**）
- 只看不动手：`sudo bash /tmp/install.sh --status`
- 指定版本：`--version v2.0.0`；改端口：`--port 8081`
- 下载慢 / 连不上：**脚本会自动换镜像重试**（ghfast.top → gh-proxy → ghproxy），
  每个源都做 sha256 校验，用哪个源装的会在日志里写清楚。
  想自己指定就加 `--mirror https://ghfast.top`

### 飞牛 fnOS

从 [Release 页](https://github.com/chenggegedeid/catflap/releases) 下载 `catflap-<版本>.fpk`，
在飞牛**应用中心 → 手动安装**里装上即可。装完桌面会出现 Catflap 图标，点开就是管理后台。

### 其它 Linux（手动）

```bash
sudo install -m 0755 catflap /usr/local/bin/catflap

# 2. 建专用用户与数据目录（权限 0700，因为 config/ 里是明文凭据）
sudo useradd --system --home /opt/catflap --shell /usr/sbin/nologin catflap
sudo install -d -o catflap -g catflap -m 0700 /opt/catflap
sudo install -d -o catflap -g catflap -m 0700 /opt/catflap/config /opt/catflap/data

# 3. 安装服务单元
sudo cp deploy/systemd/catflap.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now catflap
```

**装完立刻打开 `http://<内网IP>:3791/catflap/` 设置管理员密码。**

首次初始化**不限制来源**，局域网里任何设备的浏览器都能打开 —— 服务器没有
桌面浏览器也没关系：

```bash
curl -s http://127.0.0.1:3791/api/auth/status   # 看是否已初始化
```

> ⚠️ **在设好密码之前，能访问到这个地址的人都可以抢先设置密码**，而改密码
> 需要旧密码 —— 也就是机器会被接管。所以别把它晾在那儿。取舍的完整说明见
> [docs/decisions/0005](docs/decisions/0005-初始化接口放开来源限制.md)。

设完密码后，就可以从局域网其它设备访问了。

> **没有 IPv4 公网（运营商 CGNAT）、只有 IPv6 的家宽**，
> 建议直接走 IPv6：不走打洞、不开 UPnP，端口也不会变。
> 见 [IPv6 与单端口入口部署指南](docs/guides/IPv6-与单端口入口部署指南.md)——
> 其中也讲了怎么让所有二级域名共用一个固定端口（对外只放通一个）。

> 默认端口 **3791**（特意避开被各类 dev server / 面板当默认值的常见端口）。
> 撞上了不用挪别人：设环境变量 `CATFLAP_PORT=8081` 即可改（systemd 下加
> `Environment=CATFLAP_PORT=8081`）。改了记得同步防火墙放行规则。

### Windows

下载 `catflap-desktop.exe` 双击运行（桌面版，自动开窗），或下载命令行版在前台看日志。

### Docker

见 [`deploy/docker/`](deploy/docker/)。

> ⚠️ **不要用 root 运行。** 面板能读写 `config/`（含 DNS Token 等明文凭据）、
> 能把内网服务暴露到公网。请用专用用户 + 只读程序目录，见
> [`SECURITY.md`](SECURITY.md) 的部署安全清单。

## 从源码构建

**顺序很重要**：`app.go` 用 `//go:embed` 嵌入了两个 `dist`，**必须先构建前端**。

```bash
git clone https://github.com/chenggegedeid/catflap.git && cd catflap

(cd web/admin && npm ci && npm run build)
(cd web/home  && npm ci && npm run build)

export CGO_ENABLED=0
go build -trimpath -ldflags "-s -w -X github.com/chenggegedeid/catflap/core.Version=v1.0.0" -o bin/catflap .
```

国内建议先设模块代理：`go env -w GOPROXY=https://goproxy.cn,direct`

更多细节见 [`CONTRIBUTING.md`](CONTRIBUTING.md) 与 [`docs/`](docs/README.md)。

## 项目状态

当前为 **v1.0.0 开发阶段**：功能与上游对齐，重点在品牌剥离、安全基线与扩展架构重构。
新功能排在 v1.1 之后，路线图见 [`docs/design/`](docs/design/)。

## 参与贡献

欢迎 Issue 与 PR。动手前请先读 [`CONTRIBUTING.md`](CONTRIBUTING.md) ——
里面有**目录约定**、提交与构建流程，以及本机开发时的完整校验清单。

安全问题请**不要**开公开 Issue，走 [`SECURITY.md`](SECURITY.md) 里的私下渠道。

## 许可与致谢

- 本项目以 **GNU General Public License v3.0 or later** 发布，见 [`LICENSE`](LICENSE)。
- 引用的代码与开源项目（LinkStar、Lucky 及全部第三方依赖）的完整清单、
  许可与版权声明见 [`NOTICE`](NOTICE)。
- 名称与图标为本项目的自有资产；GPL 授予的是著作权许可，不含商标权。

<div align="center">
<sub>Catflap · 猫洞 —— 名称与图标均为自绘，与上游无关</sub>
</div>

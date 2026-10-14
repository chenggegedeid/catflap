# 更新日志

本文件记录本项目的所有重要变更。
格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

> **版本号规则（重要）**
> Release 的 tag 形如 `v1.0.0`，**不要带后缀**（如 `-beta`）。
> 程序内部解析版本时会取最后一个 `-` 之后的部分，带后缀会被解析错。
> 预发布版本请用 GitHub Release 页面的 **Pre-release** 勾选项标记。

---

## [Unreleased]

### 新增（Added）

- **新增 4 家 DNS 服务商**（共 11 家）：GoDaddy、Porkbun、HE.net（dns.he.net）、DuckDNS。
  注册表化之后加一家 = 加一个实现文件 + 一个注册文件，前端表单自动出现。
  其中 HE.net 与 DuckDNS 只支持 IPv4，界面上不会给它们选 AAAA。
- **STUN 服务器列表可一键恢复内置默认**：服务器弹窗里新增「恢复内置列表」按钮。
  之前把列表删坏（比如删空）就没有回头路，而列表一空打洞彻底起不来。
- **新增「计划任务」组件**：定时同步 DDNS 解析记录，或唤醒一台机器（每 N 分钟 /
  每天固定时刻，调度精度 30 秒）。新建的任务可以立刻「执行」一次试跑，手动与
  定时走同一条路径。在「功能管理」里可开关。
  注意：当天时刻已过的每天任务会顺延到第二天，**不补跑**（半夜关过机的话，
  早上开机不该集中触发一串任务）。
  在「功能管理」里可开关（关掉后 `/api/wol/*` 返回 404）。
  注意：接口返回的「成功」只代表包发出去了，不代表目标醒了 —— 目标没开 WOL、
  广播地址填错、或跨了网段都不会有反馈。广播地址要填**子网的广播地址**
  （如 `192.168.1.255`），不是机器 IP：机器此刻没有 IP、也不响应 ARP。

---

## [1.0.0] - 2026-10-14

首个以 **Catflap** 名义发布的版本。开发初期以开源项目 LinkStar 0.7.4 的代码为起点
（引用清单见 `NOTICE`），这一版**不新增业务功能**，做的是品牌剥离、安全基线与扩展架构。

### 变更（Changed）

- **品牌剥离**：项目由 LinkStar 更名为 **Catflap**（中文俗称「猫洞」），
  更换图标、界面文案、自签证书主题与发布产物名。
  - 管理后台路径由 `/linkstar/` 改为 **`/catflap/`**（旧书签需更新）。
  - 二进制与 Release 资产名：`catflap`、`catflap-linux-arm64`、
    `catflap-linux-armv7`、`catflap-linux-mipsle`、`catflap-desktop.exe`。
  - 浏览器本地存储键与桌面版免登录请求头随之更名
    （`catflap_token` / `X-Catflap-Desktop`）。
  - **写入 Cloudflare 的持久化标记改成本项目自己的前缀**：
    重定向规则名 `catflap:<设备>-<服务>`、入口记录注释 `catflap:entry`、
    SRV 注释 `catflap:srv`。
    本项目是独立的新项目，**只认自己的前缀**：带其它前缀（含开发初期所参考
    项目留下的 `linkstar:`）的规则会被当作「用户自己创建的」—— 不会被改写，
    删服务时也不会被回收。这是有意的取舍，理由见
    `docs/decisions/0004-改用自有-go-module-名.md` 的「不做的事」。
- 源码构建的默认版本号由硬编码的 `0.7.1` 改为 `dev`，
  发布版的版本号由 CI 在编译时注入（`-ldflags "-X github.com/chenggegedeid/catflap/core.Version=…"`）。
- **监听端口默认改为 3791**（原来是 3333）—— 3333 被太多 dev server / 面板当默认值，
  容易撞。并且新增环境变量 `CATFLAP_PORT` 可以改端口：撞上了不必去挪别人的程序。
- 界面上「这条入口记录是不是本程序建的」字段由 `byLinkStar` 改名为 `byCatflap`。

### 安全（Security）

- **首次初始化限定本机**：`POST /api/auth/setup` 现在只接受来自 `127.0.0.1` / `::1`
  的请求，杜绝「接上网线时被局域网内其他人抢先设置管理员密码」。
  初始化完成后该限制自动失效，不影响后续从局域网访问面板。
- **移除更新源中的第三方 API 镜像**：原先查最新版本时会依次尝试若干第三方
  GitHub 代理，而这些代理会同时转发 `api.github.com` 的响应 ——
  由于更新校验用的 sha256 摘要正是取自该响应，代理可同时伪造文件与摘要，
  使校验完全失效。现在**摘要只信任 GitHub 直连**；文件下载仍允许走镜像
  （文件本体有摘要兜底）。
- **收紧配置目录与文件权限**：`config/` 由 `0755` 改为 `0700`，
  配置写回时强制 `0600`。该目录内存有 DNS 服务商 Token 等明文凭据。
- **补充部署安全基线**：新增 `SECURITY.md` 部署清单与
  `deploy/systemd/catflap.service`（专用用户 + `NoNewPrivileges` +
  `ProtectSystem=strict` + 只读程序目录）。程序目录只读会**顺带禁掉一键更新**，
  这是有意的。

### 新增（Added）

- **三层扩展点架构**，解决「每加一个功能要手改四处硬编码列表」的问题：
  - `core/component`：统一组件契约 + 注册表 + **按依赖拓扑排序启动**（同一层并发、
    有依赖的严格后发）。九个业务模块（auth / home / webhook / cert / stun / ddns /
    proxy / update / system）全部接入。
    - **新增「功能管理」页**（系统与管理 → 功能管理）：可以在界面上开关各个功能。
      关闭后该功能的接口直接返回 **404**（不是 403 —— 不告诉未授权者「这里本来有
      个功能」），侧栏上的入口同时消失。改动立即生效，无需重启，状态存
      `config/components.json`。
    - 后台的路由与侧栏导航现在都由注册表/页面注册表生成：新增一个页面只需在
      `pages/registry.tsx` 里加一条记录。
    - `auth`、`home`、`system` 三个组件**刻意不支持开关**（关掉登录入口就再也进不
      去后台、关掉主页就没有访客入口、关掉系统就没有排查手段）。
  - `core/provider`：提供者注册表（DNS / 通知 / 存储），凭据用声明式 Schema 描述，
    前端表单据此自动生成 —— **新增一个服务商只需一个文件，前端零改动**。
    七家 DNS 服务商已接入，添加/编辑表单的字段与「去哪拿凭据」的链接都由后端给出。
  - `core/bus`：事件总线，把原先 `modules/stun` 里的 `RegisterOnXxx` 回调机制
    泛化为 `Publish` / `Subscribe`，模块之间不再互相 import。
    「STUN 一拿到公网 IP 就催 DDNS 推一轮」这类通知已改走总线。
- 治理文件：`NOTICE`（引用的代码与开源项目清单）、`CONTRIBUTING.md`（含目录约定与
  构建流程）、`SECURITY.md`、`CHANGELOG.md`、Issue 与 PR 模板。
- CI 与发布流水线：`.github/workflows/ci.yml`（提交即校验）、
  `release.yml`（打 tag 自动构建 5 个目标 + 飞牛 `.fpk`，并发布 Release）。
- **飞牛 fnOS 应用包**：`deploy/fnos/`（manifest、生命周期脚本、桌面入口定义）
  + `scripts/pack_fpk.py`，由 CI 在 Linux 上打包成 `.fpk`，可在应用中心手动安装。
- **Debian / Ubuntu 一键安装脚本**：`scripts/install.sh`，一行命令完成
  「识别架构 → 下载 → 校验 sha256 → 建专用用户 → 装 systemd → 开机自启」，
  支持 `--uninstall` / `--purge` / `--version` / `--dry-run`。

### 修复（Fixed）

- 发布流水线此前未注入版本号，导致源码默认版本（0.7.1）与实际产物版本
  （0.7.4）不一致：从源码构建出来的程序界面会显示错误的版本号。
  现在 CI 用 `-ldflags -X github.com/chenggegedeid/catflap/core.Version=<tag>` 统一注入。

### 说明（Notes）

- **Go module 名已改为 `github.com/chenggegedeid/catflap`**（原为 `linkstar`）。
  原先保留旧名的理由（与上游合并的冲突面最小）在定位调整后不再成立：
  本项目是独立新项目，不与上游做 `git merge`。见
  `docs/decisions/0004-改用自有-go-module-名.md`（ADR-0001 已废止）。
  一键更新认自己人的 `checkBinary()` 改用常量 `core.ModulePath`，不再写字面量。
- **版本号注入点现在是 `github.com/chenggegedeid/catflap/core.Version`**：
  版本号是构建期属性，不属于任何一层；放在 `core` 里，两侧的模块（modules/update
  需要读它、api/system_api 需要返回它）都只是读它，不会形成 import 环。
  自建脚本请同步改成后者 —— Go 链接器对 `-X` 指向的符号不存在**不报错**，
  写旧了不会失败，只会让版本号一直显示 `dev`（而版本号还参与自动更新检查）。
  CI 有一道 `scripts/check_version_ldflags.py` 守着这条。

---

## 随基线继承的变更（父提交 `9cb7cd1` 之前）

以下变更来自本仓库的**父提交链**——即 `9cb7cd1` 及其上一条 `a03c000`，
它们是 LinkStar 0.7.4 的修改版（曾以 tag `0.7.x-patched` 的形式打包为飞牛 `.fpk`）。
它们已包含在 v1.0.0 的代码中，此处补记以便追溯。
注意：这两条在 `git log` 里是**真实存在的提交**，不是被压平进某个提交的内容。

### [0.7.4-patched]

- **TCP 打洞保活加固**：上游在 CGNAT（运营商大内网）等不支持 NAT 回环的环境下，
  TCP 健康检查依赖内网自连公网地址，恒失败导致服务反复重启。
  改为在**同一条 STUN 连接上重新握手**验证映射，并在重连后**跟随新的公网端口**
  （自动重发映射事件，触发 DDNS 与入口重定向同步），不再把端口变化当致命错误。

### [0.7.2-patched]

- **STUN 服务器列表可配置**：新增 STUN 服务器管理入口
  （`GET/PUT /api/stun/servers`），可在面板里编辑服务器列表并立即重测选优。
  实测部分公共 STUN 服务会在数秒内掐断 TCP 连接，导致 TCP 打洞保活必败，
  用户可自行剔除。

---

[Unreleased]: https://github.com/chenggegedeid/catflap/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/chenggegedeid/catflap/releases/tag/v1.0.0

# 部署

按平台分类。所有方式都遵循同一条安全基线，详见 [`../SECURITY.md`](../SECURITY.md)。

```
deploy/
├─ systemd/catflap.service    Linux 服务单元（含完整安全加固）
├─ docker/                    Dockerfile 与 compose 示例
└─ README.md                  本文件
```

## 目录规划（三种方式统一）

| 路径 | 内容 | 权限 | 是否需备份 |
|---|---|---|---|
| `/usr/local/bin/catflap` | 二进制 | `0755`，**只读**（不设可写） | 否（可重新下载） |
| `/opt/catflap/` | 工作目录 | `0700`，属 `catflap` | — |
| `/opt/catflap/config/` | 配置（**含明文凭据**） | `0700` | ✅ **必须备份** |
| `/opt/catflap/data/` | 图标、壁纸、证书等 | `0700` | ✅ 建议备份 |

> **程序目录只读**是有意设计的：一键更新需要原地替换自己的二进制，目录只读会顺带
> 把它禁掉，少一条远程代码执行路径。升级流程见下面各平台小节。
>
> 权限是**两层**保障：程序自己会把 `config/`、`data/cert/` 收到 `0700`/`0600`
> （并在启动时纠正历史遗留的宽权限），systemd 单元再设 `UMask=0077` 兜底 ——
> 将来新增的写入点即使忘了设权限，也不会漏出 world-readable 的文件。

## Linux（systemd）

### 首次安装

```bash
# 1. 下载对应架构的二进制
#    资产名：catflap / catflap-linux-arm64 / catflap-linux-armv7 / catflap-linux-mipsle
sudo install -m 0755 catflap /usr/local/bin/catflap

# 2. 专用用户（不给登录 shell）
sudo useradd --system --home /opt/catflap --shell /usr/sbin/nologin catflap

# 3. 数据目录，权限 0700
sudo install -d -o catflap -g catflap -m 0700 /opt/catflap
sudo -u catflap install -d -m 0700 /opt/catflap/config /opt/catflap/data

# 4. 安装服务单元
sudo cp deploy/systemd/catflap.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now catflap
```

### ⚠️ 首次初始化必须在本机完成

服务起来之后，**不要**立刻从别的电脑打开面板 —— 程序会拒绝来自非本机的初始化请求
（防止局域网里有人抢先设置管理员密码）：

```bash
curl -s http://127.0.0.1:3791/api/auth/status   # 看 initialized 字段
```

在**服务所在的这台机器上**打开 `http://127.0.0.1:3791/catflap/` 设置密码。
设完之后，就可以从局域网其它设备访问 `http://<内网IP>:3791/catflap/` 了。

### 升级

因为 `ProtectSystem=strict` + 程序目录只读，一键更新会明确报错（这是预期的）。
手动升级：

```bash
sudo systemctl stop catflap
sudo install -m 0755 catflap-new /usr/local/bin/catflap
sudo systemctl start catflap
```

> 配置与 `data/` 不受影响，无需重新配置。

### 常见问题

| 现象 | 原因 |
|---|---|
| 启动即退出，日志报 `permission denied` 写配置 | `WorkingDirectory` 没设，或它指向了只读路径 |
| 保存的配置重启后丢失 | 同上 —— 程序按**工作目录**读写 `config/` |
| 从别的机器打开面板显示连接被拒 | 面板只监听 `0.0.0.0:3791`，检查防火墙与 `RestrictAddressFamilies` |
| 一键更新报「请手动替换」 | 正常，见上面升级小节 |
| 证书申请失败 | 检查 `ProtectSystem` 下证书目录是否在 `ReadWritePaths` 内 |

## Docker

见 [`docker/`](docker/)。要点：

- 容器内不要用 root 跑（Dockerfile 里已建非 root 用户）
- `config/` 与 `data/` 必须挂卷，否则容器重建就丢配置
- STUN 打洞需要主机网络或足够的端口映射，`network_mode: host` 是最省事的选择

## Windows

下载 `catflap-desktop.exe` 双击运行。首次运行会在**程序所在目录**创建 `config/` 与
`data/`，所以别把它放在 `C:\Program Files` 下（不可写）。

命令行版 `catflap.exe` 在前台运行并输出日志，适合排查问题。

## 配置放在哪

配置与数据都在**程序所在目录**下的 `config/` 与 `data/`。systemd 部署时是
`/opt/catflap/config` 与 `/opt/catflap/data`，备份这两处即可。

> 本项目的配置结构**与参考的上游项目保持一致**，但两者是不同的产品、没有
> 互相升级的关系：不要直接把别的程序的配置目录拷过来当迁移用。真要搬，
> 请按上面的目录逐个核对，别指望「拷过去就能用」。

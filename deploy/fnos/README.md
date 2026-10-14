# 飞牛 fnOS 应用包（.fpk）

`deploy/fnos/` 是打飞牛第三方应用安装包需要的全部素材，`scripts/pack_fpk.py`
负责把它和 Go 二进制打成一个 `.fpk`。

## 怎么打

```bash
# 在仓库根目录
python3 scripts/pack_fpk.py                 # 版本取 deploy/fnos/manifest
python3 scripts/pack_fpk.py --version 1.0.1
python3 scripts/pack_fpk.py --arch arm64
python3 scripts/pack_fpk.py --binary <已编好的 linux 二进制>
```

产物在 `dist/catflap-<版本>.fpk`。

> **必须在 Linux（或 CI 的 ubuntu runner）上打**，原因有两个：
> ① 包里的二进制必须是 linux 的；② Windows 上 `tarfile` 打出来的权限位不可靠，
> 而 `cmd/*` 没有可执行位的话，装完应用**起不来**。
> 本机（Windows）的 Go 工具链放在同步盘上，linux 平台的 stdlib 源文件是云端占位，
> 交叉编译会失败 —— 所以 fpk 由 CI 构建，见 `.github/workflows/release.yml` 的
> `fnos-fpk` 任务。

## 包的结构

```
外层 fpk（tar.gz）
  app.tgz           ← 内层，必须是真 gzip
  cmd/*             ← 生命周期脚本（必须可执行）
  config/privilege
  config/resource
  wizard/           ← 参照包里是空目录，保持一致
  ICON.PNG
  ICON_256.PNG
  manifest          ← checksum = md5(app.tgz)

内层 app.tgz（tar.gz）
  server/catflap    ← Go 二进制
  ui/config         ← 桌面图标定义
  ui/images/*.png   ← 取自仓库根目录的 icon_64.png / icon_256.png
  config/*
```

## 飞牛注入的环境变量

| 变量 | 用途 |
|---|---|
| `TRIM_APPDEST` | 应用安装目录（只读，升级会被整包替换） |
| `TRIM_PKGVAR` | 持久目录（升级/重装保留）—— `cmd/main` 先 `cd` 到这里再启动，所以 `config/`、`data/`、`logs/` 都落在它下面 |

## 容易踩的三个坑（打包脚本里都有断言守着）

1. **内层 `app.tgz` 必须是 gzip 流**。写成裸 tar 时，飞牛会报
   「应用包格式不符合系统版本要求」，而 manifest 层面完全看不出来。
2. **`cmd/*` 必须带可执行位**，否则装上起不来。
3. **端口要三处一致**：`manifest.service_port`、`ui/config` 的 `port`、
   以及本文件里 `SERVICE_PORT` 常量的约定（当前都是 **3791**）。
   另外 `ui/config` 的键名必须等于 `manifest` 的 `desktop_applaunchname`
   （`Catflap.Application`），否则装完**没有桌面图标**。

## 改端口

同时改三处：`deploy/fnos/manifest` 的 `service_port`、`deploy/fnos/ui/config` 的
`port`、`scripts/pack_fpk.py` 的 `SERVICE_PORT`。打包脚本会断言三者一致，漏一处会直接失败。

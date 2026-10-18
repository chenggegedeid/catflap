# 贡献指南

感谢愿意帮忙。动手前请先花两分钟读完这一页 —— 尤其是**改动时注意**那一节。

## 目录约定

> **存量目录冻结，增量严格分类。**

Go 的包路径**就是**目录路径。把 `modules/stun/` 挪进 `modules/network/stun/` 会让全库
的 import 全部失效。所以：

- `api/`、`modules/`、`routers/`、`middleware/`、`utils/` 的结构**保持现状**，不要重排；
- 新增内容**必须**按功能放到对应分类目录：
  | 要加的东西 | 放哪 |
  |---|---|
  | 一个新的业务功能 | `modules/<功能名>/` + `api/<功能名>_api/` + `routers/<功能名>_routers.go` |
  | 复用型框架能力（注册表、总线、调度） | `core/<能力名>/` |
  | 一家新的 DNS / 通知 / 存储服务商 | 只加一个 `register.go`，不要改 `switch`（见下） |
  | 品牌资产（图标、色板） | `branding/`，这是唯一来源 |
  | 部署物料（unit、Dockerfile、脚本） | `deploy/` |
  | 文档 | 决策进 `docs/decisions/`、指南进 `docs/guides/`、设计与路线图进 `docs/design/` |
- **不要往根目录丢笔记、临时脚本、截图**——笔记进 `docs/`，截图进 `docs/img/`，
  一次性脚本用完就删。

## 开发环境

```bash
go version          # 需要 1.25.1（见 go.mod）
node --version      # 需要 >= 20（前端用 Vite 8）
```

国内建议设模块代理：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

## 构建

**顺序很重要**：`app.go` 用 `//go:embed` 嵌入了 `web/admin/dist`，
所以**必须先构建前端，再编译后端**，否则 `go build` 直接失败。

```bash
# 前端
(cd web/admin && npm ci && npm run build)

# 后端：命令行版
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/catflap .

# 后端：Windows 桌面版（带 desktop 构建标签）
go build -tags desktop -trimpath -ldflags "-s -w" -o bin/catflap-desktop.exe .
```

装了 [`task`](https://taskfile.dev) 的话：

```bash
task build:frontend   # 先跑这个
task build            # 桌面版 + Windows CLI + Linux CLI
```

## 提交前自检

```bash
gofmt -l .            # 必须无输出
go vet ./...          # 必须无报错
go test ./...         # 必须通过
```

改了前端的话，别忘了 **重新构建 dist 并提交**（dist 是入库的，原因见 `.gitignore` 里的说明）。

## 改动时注意

- **品牌相关**（名称、图标、文案、颜色）：集中改 `branding/`、`app.go` 顶部常量、
  前端 `index.css` 的主题 token。**不要散落在各页面里硬编码。**
- **Go module 路径不要顺手改**：现在是 `github.com/chenggegedeid/catflap`。
  改它要同步三处 —— 全库 import、`core.ModulePath`（`checkBinary()` 用它认
  自己人）、以及发布流水线里的 `-X` 注入点。后两者漏改都不会报错，
  只会让「一键更新拒绝所有合法文件」和「发布版版本号一直是 dev」。
  理由与回退方式见 [`docs/decisions/0004`](docs/decisions/0004-改用自有-go-module-名.md)。
- **行尾**：仓库里**所有文本文件都是 LF**
  （见 [`docs/decisions/0002-行尾统一为-LF.md`](docs/decisions/0002-行尾统一为-LF.md)）。
  clone 之后请确认：

  ```bash
  git config --get core.autocrlf    # 应为 false 或 input，不应是 true
  git config core.autocrlf false    # 如果是 true，改成 false
  ```

  `.gitattributes` 里已经写死 `eol=lf`，所以即使忘了设也**不会**污染入库，
  但本地 `git status` / `git diff` 会显示一堆无关的行尾变更，很难看清自己改了什么。
  Windows 上 Git for Windows 安装器的默认值就是 `true`，所以这一步经常需要。

## 新增一个服务商（DNS / 通知 / 存储）

**不要**去改 `modules/ddns/dns/dns_build_provider.go` 里的 `switch` —— 那是旧写法。
用 `core/provider` 的注册表：新建一个文件调用 `provider.Register(...)`，
把凭据字段声明在 `Descriptor.Credentials` 里，前端表单会自动生成。

> 该注册表见 `core/provider/`。迁移期两套机制并存，新增一律走注册表。

## 许可与合规

- 本项目是 **GPL-3.0-or-later**，你提交的代码同样以该许可发布。
- **不要**从许可不兼容的项目里复制代码。特别是：
  - `snltty/linker` 是 **GPL-2.0，与 GPL-3.0 不兼容**，只能参考设计、不能借代码；
  - Lucky 的 **v2/v3 是闭源加壳**的，**不得**逆向、复制名称或图标；
    v1.4.10 及更早是 MIT，若要移植必须更新 `NOTICE`。
- 若你的改动移植了任何第三方代码，**必须在 `NOTICE` 里补上版权声明**。

## Issue 与 PR

- Bug 请附：版本号、平台与架构、复现步骤、相关日志（记得**打码**域名与 Token）。
- PR 请：一个 PR 只做一件事；描述里写清「为什么」而不只是「改了什么」；
  涉及界面改动的附截图。

## 品牌改动

改名称 / 图标 / 颜色这类事，会影响 README、界面、发布资产名、证书 CN 等**十几处**，
请先开 Issue 讨论，别直接提 PR。

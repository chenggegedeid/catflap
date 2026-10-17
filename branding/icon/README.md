# 位图图标（.ico / .png）

`favicon.svg` 可以直接用，现代浏览器都支持 SVG favicon。
但下面三处**必须**是位图，需要从矢量母版导出：

| 产物 | 用途 | 需要的尺寸 |
|---|---|---|
| `catflap.ico` | Windows 桌面版图标（`wails3 generate syso -icon` 消费它） | 16 / 32 / 48 / 256 |
| `favicon.ico` | 老浏览器兜底（放 `web/admin/public/`） | 16 / 32 / 48 |
| `catflap-512.png` | 应用商店、社交卡片、README 备用 | 512 |

## 状态

✅ **已生成**（2026-10-14）。上游的 `_待替换-上游图标.ico` 已删除，商标风险解除。

当前落位：

| 产物 | 位置 | 尺寸 |
|---|---|---|
| 桌面版 ICO | `build/windows/icon.ico` + `branding/icon/catflap.ico` | 16/24/32/48/64/128/256 |
| Wails 源图 | `build/appicon.png` | 1024 |
| 后台 favicon | `web/admin/public/favicon.png` | 128 |
| 后台侧边栏 logo | `web/admin/src/assets/logo.png` | 128 |

> 以前还有一行「导航页 favicon」指向 `web/home/public/favicon.png`，
> 导航主页已在 v2.1.0 之后移除。

## 生成方法（本仓库实际用的那条）

Node + `sharp` + `png-to-ico`，脚本见
`~/.workbuddy/binaries/node/workspace/icogen/gen_icons.mjs`（本机私有，不入库）。
核心就是一句话：把 512 的 viewBox 按 `density = 72 × 目标尺寸 / 512` 渲染，
再缩到目标尺寸。改了 SVG 母版后重跑一次即可。

其它可选工具（Inkscape / ImageMagick / svgexport）保留在下面，离线时用得上。

源文件统一用 `branding/logo/catflap.svg`（512×512）。

### 方法一：Inkscape（推荐，保真度最好）

```bash
inkscape branding/logo/catflap.svg --export-type=png --export-width=256 \
  --export-filename=branding/icon/catflap-256.png
inkscape branding/logo/catflap.svg --export-type=png --export-width=512 \
  --export-filename=branding/icon/catflap-512.png
```

### 方法二：ImageMagick

```bash
magick -background none -density 512 branding/logo/catflap.svg -resize 512x512 \
  branding/icon/catflap-512.png
# 多尺寸 ICO 一次生成
magick branding/icon/catflap-512.png -define icon:auto-resize=256,48,32,16 \
  branding/icon/catflap.ico
```

### 方法三：Node（不装桌面软件）

```bash
npx svgexport branding/logo/catflap.svg branding/icon/catflap-512.png 512:512
```

### 方法四：纯 Python（无外部依赖）

用 `zlib` + `struct` 手写 PNG，自行光栅化 SVG 里的几何图形。
本项目不提供该脚本 —— 图标图形简单，但手写光栅化容易在抗锯齿上出偏差，
不如直接用上面三种工具。

## 生成后要放到哪

```bash
cp branding/icon/catflap.ico        build/windows/icon.ico        # 桌面版图标
cp branding/icon/favicon.ico        web/admin/public/favicon.ico  # 浏览器兜底
cp branding/icon/catflap-512.png    docs/img/logo.png             # 文档配图
```

`web/admin/public/favicon.png`（上游留下的 18KB 位图）也要一并替换为新图标，
并在 `index.html` 里把 favicon 指向 `favicon.svg`（带 `favicon.ico` 兜底）：

```html
<link rel="icon" type="image/svg+xml" href="/catflap/favicon.svg">
<link rel="alternate icon" href="/catflap/favicon.ico">
```

## 验收

- [x] 上游图标已删除，`branding/icon/catflap.ico` 落位到 `build/windows/icon.ico`
- [ ] 16px 下仍能认出「一扇门 + 一个洞」（贴在资源管理器/任务栏里看一眼）
- [ ] 浅色与深色背景下都不糊
- [ ] 仓库里搜不到任何上游 LinkStar 的图标资源

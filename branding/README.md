# 品牌资产

**这里是品牌资产的唯一来源。** 改名称、图标、配色，只动这个目录，
不要在页面里散写十六进制色值 —— 否则以后调一次色要全库搜。

## 目录内容

```
branding/
├─ logo/
│  ├─ catflap.svg            主图标：门上猫洞（512×512 矢量母版）
│  ├─ catflap-mono.svg       单色版：无深色底、纯 #2563EB 描边（浅底 / 印刷 / 文档用）
│  └─ catflap-wordmark.svg   横版文字标：图标 + 「Catflap」+ 副标题
├─ icon/
│  ├─ favicon.svg            浏览器图标（现代浏览器直接支持 SVG favicon）
│  └─ README.md              位图（.ico / .png）的生成步骤
└─ README.md                 本文件
```

## 主图标讲的是什么

一扇**门** = 你家网络的边界（NAT / 路由器）；
门下那个**发光的拱形活门** = 猫洞本身；
一条线从洞里**穿出去**接到外面的节点 = 内网服务被访问到。

一个图标讲完「不拆门，也能让内网出去」这件事。

## 色板（冻结）

| 用途 | 色值 | 说明 |
|---|---|---|
| 底 | `#0A1A2F` → `#0A3055` | 墨蓝，图标与深色界面背景 |
| 门体 | `#2E6BB8` → `#14406F` | 门的渐变 |
| 洞口描边 | `#3B82F6` | |
| 主蓝 | `#2563EB` | 品牌主色、链接、主按钮、单色版 |
| 内网线 | `#60A5FA` | 洞内拓扑 |
| 天蓝 | `#7DD3FC` | 连接线 |
| 青 | `#22D3EE` | 「通了」的强调色、外圈节点 |
| 冰白 | `#E0F2FE` | 节点、高光 |

**为什么是蓝色**：这个品类的用户（NAS、软路由、家庭服务器）大多数时候开着深色界面，
而且蓝色在「网络 / 连接 / 工具」这个语义场里最不容易出错。刻意避开了上游 LinkStar 的
蓝紫渐变与 Lucky 的橙粉 —— 一眼能区分开，是独立项目的必要条件。

## 各处的用法

| 场景 | 用哪个 | 尺寸 |
|---|---|---|
| 浏览器标签页 | `icon/favicon.svg` | 16–32px |
| 面板侧边栏 | `logo/catflap.svg` | 36px |
| 导航主页头部 | `logo/catflap.svg` | 48px |
| README / 官网横幅 | `logo/catflap-wordmark.svg` | 宽度自适应 |
| 深色底、纯色印刷 | `logo/catflap-mono.svg` | ≥32px |
| Windows 桌面版图标 | `icon/catflap.ico`（待生成） | 16/32/48/256 |

> ⚠️ **单色版不要用在 16px。** 它没有深色底，洞口与洞内的拓扑线条会连成一块。
> 最小可用尺寸是 32px。

## 改品牌时要一起改的地方

图标和名称不只在 `branding/` 里。换品牌时**必须同步**下面这些位置，
漏一个就会在某个角落露馅：

- **前端**：`web/admin/src/assets/` 与 `web/admin/public/favicon.png`、
  `web/home/src/assets/`；界面文案里的品牌名
- **后端**：`app.go` 顶部常量、`middleware/auth.go` 的免登录头名、
  `modules/cert/selfsign.go` 的自签证书主题（**浏览器里能直接看到**）
- **构建**：`build/windows/icon.ico`、`build/windows/info.json`、
  `Taskfile.yml` 的 `APP_NAME`
- **发布**：`.github/workflows/release.yml` 里的资产名，
  且**必须与 `modules/update/update.go` 的 `assetName()` 完全一致**
- **文档**：README、NOTICE、CHANGELOG

完整清单见 `docs/design/` 下的品牌章节。

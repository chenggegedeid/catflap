# 文档索引

本项目的文档按**用途**分类，不按时间。

| 目录 | 放什么 | 什么时候看 |
|---|---|---|
| [`decisions/`](decisions/) | **架构决策记录（ADR）** —— 已经定下来的、会影响后续所有工作的决定，含当时的取舍与代价 | 想改全库约定、想「顺手修好」某件事之前 |
| [`design/`](design/) | 品牌与架构设计、功能路线图 | 想知道「为什么要做成这样」「下一步做什么」 |
| [`guides/`](guides/) | **按场景的操作指南**（怎么在某种网络环境里装好、配好） | 要落地部署，尤其是没有 IPv4 公网 / CGNAT 的环境 |

## 建议的阅读顺序

- **刚接手这个项目** → 根目录 [`README.md`](../README.md) → [`../CONTRIBUTING.md`](../CONTRIBUTING.md)
  （尤其是其中的**目录约定**与**改动时注意**）→ [`decisions/`](decisions/)
- **准备部署** → [`../deploy/README.md`](../deploy/README.md) → [`guides/快速上手.md`](guides/快速上手.md) →
  [`guides/部署与升级.md`](guides/部署与升级.md) → [`../SECURITY.md`](../SECURITY.md)
- **没有 IPv4 公网（CGNAT）/ 只有 IPv6** → [`guides/IPv6-与单端口入口部署指南.md`](guides/IPv6-与单端口入口部署指南.md)
  （这条路线不需要打洞、不需要 UPnP，端口也不会变）
- **准备改代码** → [`design/`](design/) 的架构部分 → [`../core/README.md`](../core/README.md)
- **准备发版** → [`design/路线图.md`](design/路线图.md) →
  [`../.github/workflows/release.yml`](../.github/workflows/release.yml)

## 一条建议先看的决策

[全库行尾统一为 LF](decisions/0002-行尾统一为-LF.md) ——
这条**更正过一版错误结论**。它值得看的不是结论，而是「一个很有道理的错判是怎么产生的」：
当时用 HTTP 响应去推断仓库的存储形态，而正确做法是去读 git 对象库。
附带一条通用教训：**看到「几乎所有文件都异常」时，先怀疑本地配置，而不是先怀疑远端。**

## 许可与第三方代码

政策见根目录 [`NOTICE`](../NOTICE)（来源声明与商标）与 [`../CONTRIBUTING.md`](../CONTRIBUTING.md)
的「许可与合规」一节（引入外部代码的边界）。

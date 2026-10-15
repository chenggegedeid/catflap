# 文档索引

本项目的文档按**用途**分类，不按时间。

> 曾经这里有条「上游区冻结」的规则：`about.md`、`快速上手.md` 等上游原始路径
> 不许动，为的是将来还能 `git merge` 上游。自 [ADR-0004](decisions/0004-改用自有-go-module-名.md)
> 放弃与上游合并、[ADR-0006](decisions/0006-移除上游区目录约束.md) 清理上游遗留之后，
> 这条约束已撤销——`docs/` 现在只有一套按用途分类的目录。

本项目各目录分工如下：

| 目录 | 放什么 | 什么时候看 |
|---|---|---|
| [`decisions/`](decisions/) | **架构决策记录（ADR）** —— 已经定下来的、会影响后续所有工作的决定，含当时的取舍与代价 | 想改全库约定、想「顺手修好」某件事之前 |
| [`design/`](design/) | 品牌与架构设计、功能路线图 | 想知道「为什么要做成这样」「下一步做什么」 |
| [`compliance/`](compliance/) | 许可、来源归属、合规边界、第三方代码政策 | 要引入外部代码、要对外发布、收到许可相关质询 |
| [`guides/`](guides/) | **按场景的操作指南**（怎么在某种网络环境里装好、配好） | 要落地部署，尤其是没有 IPv4 公网 / CGNAT 的环境 |
| [`upstream/`](upstream/) | 上游相关**存档**（被本项目改写过的那份原始 README） | 追溯「上游原来是怎么写的」 |


## 建议的阅读顺序

- **刚接手这个项目** → 根目录 [`README.md`](../README.md) → [`../CONTRIBUTING.md`](../CONTRIBUTING.md)
  （尤其是其中的**目录约定**与**上游同步策略**）→ [`decisions/`](decisions/) 两条 ADR
- **准备部署** → [`../deploy/README.md`](../deploy/README.md) → [`guides/快速上手.md`](guides/快速上手.md) →
  [`guides/部署与升级.md`](guides/部署与升级.md) → [`../SECURITY.md`](../SECURITY.md)
- **没有 IPv4 公网（CGNAT）/ 只有 IPv6** → [`guides/IPv6-与单端口入口部署指南.md`](guides/IPv6-与单端口入口部署指南.md)
  （这条路线不需要打洞、不需要 UPnP，端口也不会变）
- **准备改代码** → [`design/`](design/) 的架构部分 → [`../core/README.md`](../core/README.md)
- **准备发版** → [`design/路线图.md`](design/路线图.md) → [`compliance/`](compliance/) →
  [`../.github/workflows/release.yml`](../.github/workflows/release.yml)

## 两条必须先看的决策

这两条 ADR 都关于**「与上游保持可合并」**，而且两条都反直觉，动手前务必看：

1. [保留 Go module 名 `linkstar`](decisions/0001-保留-go-module-名.md) ——
   「顺手改成 catflap」看起来天经地义，代价是**永久失去与上游同步的能力**，
   并且会让一键更新拒绝所有合法二进制
2. [全库行尾统一为 LF](decisions/0002-行尾统一为-LF.md) ——
   这条**更正过一版错误结论**。它值得看的不是结论，而是「一个很有道理的错判是怎么产生的」：
   当时用 HTTP 响应去推断仓库的存储形态，而正确做法是去读 git 对象库。
   附带一条通用教训：**看到「几乎所有文件都异常」时，先怀疑本地配置，而不是先怀疑上游。**

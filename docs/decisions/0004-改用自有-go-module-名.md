# 0004. 改用自有 Go module 名 `github.com/chenggegedeid/catflap`

- 状态：已采纳
- 日期：2026-10-14
- 影响面：全库（196 个文件 / 441 行 import）
- 取代：[0001](0001-保留-go-module-名.md)

## 处境

[ADR-0001](0001-保留-go-module-名.md) 决定保留 `module linkstar`，理由有三条：

1. 改名要动 168 个文件的 import（机械成本）；
2. `checkBinary()` 用 `buildinfo.Path == "linkstar"` 认自己人（一处常量）；
3. **保留原名，`git merge upstream/main` 的冲突面才会收敛到「真正改过的文件」** ——
   原文写的是「这决定这个分支能活多久」。

前两条是体力活，第三条才是当初真正的权衡。而第三条的前提是：**我们要持续吸收
上游的提交**。

## 前提消失

方向调整后，本项目的定位发生了变化：

- 它是一个**独立的新项目**，有自有名称、图标与功能路线，不再与上游互相升级；
- 上游后续若有更新，采用**人工阅读、借鉴实现思路**的方式吸收，不做 `git merge`；
- 相应地，仓库历史也不再需要保留上游的提交（见 `NOTICE` 第 1 节的说明）。

于是第三条理由不成立，剩下的只有「机械成本」—— 而机械成本是一次性的。

## 决定

改用与仓库 URL 一致的 module 路径：

```
module github.com/chenggegedeid/catflap
```

配套改动：

- 196 个文件的 441 行 import（`sed 's|"linkstar/|"github.com/chenggegedeid/catflap/|g'`）；
- 新增 `core.ModulePath` 常量，`checkBinary()` 改为与它比较 ——
  **不再写字面量**，避免将来改名时又漏掉一处；
- 发布流水线的 `-X` 注入点、README、CHANGELOG 同步；
- `scripts/check_version_ldflags.py` 的匹配模式同步。

## 代价与风险

1. **放弃 `git merge upstream/main`**。以后每个文件都与上游不同，三方合并不可用。
   接受：本就不打算合并，改为人工借鉴。
2. **改名时最容易漏的是「看不见」的地方**。Go 链接器对 `-X` 找不到符号**不报错**，
   所以注入点漏改的表现是「发布版版本号一直是 dev」，而版本号还参与自动更新
   检查 —— 等于连更新一起坏掉。这个坑本项目已经踩过一次（M3 期间发现
   `release.yml` 两处仍指向旧符号）。因此：
   - `core.ModulePath` 做成常量，消除第二类漏改；
   - CI 增加 `scripts/check_version_ldflags.py`，把注入点变成可被检查的断言。
3. 若将来仓库改名/迁移，import 路径需要再改一次。属于可接受的一次性成本。

## 不做的事

**不保留旧前缀的数据兼容。** 写进用户 DNS 服务商账号的规则/记录标记
（重定向规则名、入口记录注释、SRV 注释）现在只认 `catflap:` 前缀，不再认
上游时代的 `linkstar:` 前缀。

- 理由：本项目是独立新项目，不存在「老用户升级上来」这条路径。
- 后果：带有旧前缀的规则会被当作「用户自己创建的」—— 不会被就地改写，
  删服务时也不会被回收。这是有意的：宁可不去动不认识的规则，也不要因为
  「像是自己写的」就改掉别人的东西（重定向规则按顺序匹配，改错了会让
  访问落到失效端口上，而且面板里不会有任何提示）。
- 由 `TestSameRuleKeyClaimsOnlyOwnPrefix`、`TestMergeLeavesForeignRuleAlone`、
  `TestRemoveOnlyFindsOwnRule` 三条测试钉住这个行为。

## 如何回退

把 `go.mod` 第一行、`core.ModulePath`、以及 `-X` 注入点改回 `linkstar`，
并对 .go 文件做一次反向 sed 即可（`test/` 目录不在改名范围内，无需处理）。
若要恢复旧前缀兼容，撤销 `isOwnEntryComment` / `isOwnSRVComment` /
`ruleKeyPrefixes` 三处并改回对应的 4 条测试。

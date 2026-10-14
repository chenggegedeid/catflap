package core

// ModulePath 本项目的 Go module 路径，与 go.mod 第一行保持一致。
//
// 为什么做成常量：一键更新的 checkBinary() 靠 buildinfo 里的 Path 认「这是不是
// 我们自己的二进制」，那个值正是 module 路径。写成散落的字符串字面量，改
// module 名时就一定会漏——本项目已经漏过一次（release.yml 里两处还写着旧名，
// 而 Go 链接器对 -X 找不到符号不报错，发布版会静默变回 dev）。
const ModulePath = "github.com/chenggegedeid/catflap"

// Version 当前版本号。
//
// # 为什么它放在 core 而不是某个 API 包
//
// 版本号是**构建期属性**，读取它的人遍布各层：界面要显示、更新模块要拿它跟
// GitHub 上的最新版比对、日志要写进首行。放在 api/system_api 里就会出现
// 「模块为了知道自己是哪个版本，得去 import 一个 API 包」这种倒挂 ——
// 而 API 包又是 import 模块的，直接成环。
//
// # 注入方式
//
// 是 var 而不是 const，为了能在编译时覆盖、不用改源码：
//
//	go build -ldflags "-X github.com/chenggegedeid/catflap/core.Version=v1.0.0"
//
// ⚠️ 注意：Go 链接器对**找不到的符号不报错**，名字写错会静默失效、
// 版本号一路显示 "dev"。改注入路径时记得同步 build 脚本与文档 ——
// CI 里有一道 scripts/check_version_ldflags.py 专门守这条。
var Version = "dev"

// module 名已改为本项目的自有路径（与仓库 URL 一致）。
//
// 它曾经是 linkstar —— 那是为了让 `git merge upstream/main` 的冲突面收敛到
// 「真正改过的文件」。现在项目已明确为独立项目、不再与上游做 git 合并
// （上游有更新时人工借鉴代码），那条理由不成立了。见 docs/decisions/0001
// 中的「已废止」说明与 0004。
//
// ⚠️ 改这个值时必须同步：
//   - core.ModulePath（core/version.go）—— 一键更新靠它认自己人；
//   - 发布流水线里的 -X 注入点 —— Go 链接器对找不到符号**不报错**，
//     写旧了发布版会一直显示 dev。CI 有 scripts/check_version_ldflags.py 守着。
module github.com/chenggegedeid/catflap

// 为什么精确到补丁版本而不是写 go 1.26.0：
// CI 用 go-version-file: go.mod，装的是这里写的版本。
// 这 47 个漏洞的修复落在 go1.26.9 —— 写 1.26.0 的话 CI 装 1.26.0，
// 标准库的补丁就不会被打进二进制，扫出来照样是一堆洞。
go 1.26.9

require (
	github.com/gin-gonic/gin v1.11.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/huin/goupnp v1.3.0
	github.com/libp2p/go-reuseport v0.4.0
	github.com/lionsoul2014/ip2region/binding/golang v0.0.0-20261014021221-1bf6cabfb63c
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/pion/stun v0.6.1
	github.com/pion/stun/v3 v3.1.7
	github.com/sirupsen/logrus v1.9.4
	github.com/wailsapp/wails/v3 v3.0.0-beta.23
	golang.org/x/crypto v0.57.0
	golang.org/x/net v0.60.0
	golang.org/x/sync v0.23.0
	golang.org/x/sys v0.48.0
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/bytedance/sonic v1.14.0 // indirect
	github.com/bytedance/sonic/loader v0.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/gabriel-vasile/mimetype v1.4.8 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.27.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pion/dtls/v2 v2.2.12 // indirect
	github.com/pion/dtls/v3 v3.1.14 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/transport/v2 v2.2.4 // indirect
	github.com/pion/transport/v4 v4.1.0 // indirect
	github.com/pion/transport/v5 v5.0.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/arch v0.20.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/protobuf v1.36.9 // indirect
)

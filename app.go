//go:generate python scripts/gen_module_imports.py

package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"
	// 下面四个是跨模块注入用到的：cert / stun 暴露注册接口，
	// ddns 与 dns 提供被注入的实现（见 registerCertDNSSolver 的说明）。
	"github.com/chenggegedeid/catflap/modules/cert"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/modules/ddns/dns"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/routers"
	"github.com/chenggegedeid/catflap/utils/utilsFile"

	// 副作用导入：把所有模块包加载进来，让它们 init() 里的
	// component.MustRegister 得到执行。这个文件是生成的（见 //go:generate），
	// 所以新增一个模块只需要新增目录，不用回来改这里。
	_ "github.com/chenggegedeid/catflap/modules/all"

	"github.com/sirupsen/logrus"
)

const appName = "Catflap"

// 三个地址都由 core 给出的回环 URL 拼出来（core/listen.go）：
// 端口只有一处定义，改端口不会漏掉「探测后端在不在」那一步。
var (
	backendURL = core.LoopbackURL()
	adminURL   = backendURL + "/catflap/"
	homeURL    = backendURL + "/"
)

//go:embed web/admin/dist web/home/dist
var webFS embed.FS

func initRuntime() {
	// 设置时区
	os.Setenv("TZ", "Asia/Shanghai")
	core.InitLogger()
	logrus.Info("Catflap Run")

	// 收紧敏感目录权限：config 存 JWT 签名密钥与各平台 Token，data/cert 存站点私钥，
	// logs 里有内网 IP 与访问路径。老版本用 0755/0644 建的，升级后第一次启动在这里一次性纠正。
	// 失败只记警告不阻塞启动——目录可能是只读挂载，那属于部署方主动的选择。
	for _, dir := range []string{"config", cert.DataDir, "logs"} {
		if err := utilsFile.SecureTree(dir); err != nil {
			logrus.Warnf("收紧 %s 目录权限失败：%v", dir, err)
		}
	}

	core.ListenShutdown()
}

func startBackend(webFS embed.FS) (bool, error) {
	if isBackendReady() {
		logrus.Infof("后端已在 %s 运行", backendURL)
		return false, nil
	}

	go routers.Run(webFS)

	if err := waitForBackend(); err != nil {
		return false, err
	}

	startModulesInBackground()
	return true, nil
}

// waitForBackend 轮询等待后端就绪，超时返回 error。
func waitForBackend() error {
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("后端在 %s 未能就绪", backendURL)
		case <-ticker.C:
			if isBackendReady() {
				return nil
			}
		}
	}
}

// isBackendReady 探测本机后端是否已经启动并返回 Catflap 管理后台页面。
func isBackendReady() bool {
	conn, err := net.DialTimeout("tcp", core.LoopbackAddr(), 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()

	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(adminURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), "Catflap")
}

// startModulesInBackground 后台初始化各业务组件。
//
// 这里只剩两件事，别的一律交给注册表：
//
//   - 连接跨模块注入（见下面两个函数，它们是循环依赖的破环点，绕不开）；
//   - 调 component.InitAll 按依赖顺序把组件跑起来。
//
// 组件顺序、失败处理、依赖被跳过这些事情全在注册表里，新增模块不用改这里。
func startModulesInBackground() {
	registerCertDNSSolver()
	registerSTUNRedirectSyncer()

	go func() {
		// 整批给 5 分钟：stun 要认出口网卡、逐个测 STUN 服务器，是秒级的网络
		// 操作，不该被单个组件的默认上限掐掉。单组件超时另有 narrower 的闸门，
		// 见 component.DefaultInitTimeout。
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := component.InitAll(ctx); err != nil {
			// 每个组件失败的细节已经由注册表分别记了日志，这里只留一条总数，
			// 免得在几百行启动日志里看不出「到底有没有东西起不来」。
			logrus.Errorf("有组件初始化失败：%v", err)
		}
	}()
}

// registerCertDNSSolver 把 DDNS 的 TXT 写入能力注入证书模块。
//
// modules/ddns 已经 import 了 modules/stun，而 stun 要 import cert，
// 所以 cert 必须是叶子包、不能直接 import ddns。cert 只定义一个窄接口，
// 由这里（同时能看见两边）在启动时注入实现，依赖图保持无环。
//
// 工厂体内部到真正签发时才会调用 ddns，所以此处同步注册不依赖 DDNS 已初始化。
func registerCertDNSSolver() {
	cert.RegisterDNSSolverFactory(func(id uint) (cert.TXTWriter, error) {
		p, ok := ddns.Runtime.FindProvider(id)
		if !ok {
			return nil, fmt.Errorf("DNS 服务商 %d 不存在，请先在 DDNS 里配置", id)
		}
		w := dns.BuildACMEClient(p)
		if w == nil {
			return nil, fmt.Errorf("服务商 %s 暂不支持 DNS-01", p.Type)
		}
		return w, nil
	})
}

// registerSTUNRedirectSyncer 把 DDNS 的重定向规则能力注入 STUN。
//
// 方向和上面那个一样：ddns 已经 import 了 stun，stun 不能反过来 import ddns，
// 所以 stun 只声明一个窄接口，由这里在启动时注入实现。
func registerSTUNRedirectSyncer() {
	// 「STUN 一拿到公网 IP 就催 DDNS 推一轮」这条通知已经改走事件总线：
	// 发布方是 modules/stun（notifyPublicIPChanged），订阅方是 modules/ddns
	// （ddns_events.go），两边都只认 bus.TopicPublicIPChanged。这里不再需要
	// 把两者接起来 —— 这也是为什么这个函数里剩下的都是「工厂注入」而不是
	// 「事件订阅」：工厂是「谁提供实现」的静态组装，事件是「发生了什么」的
	// 动态通知，后者本就该由总线承担。
	stun.RegisterRedirectSyncerFactory(func(id uint) (stun.RedirectSyncer, error) {
		p, ok := ddns.Runtime.FindProvider(id)
		if !ok {
			return nil, fmt.Errorf("DNS 服务商 %d 不存在，请先在 DDNS 里配置", id)
		}
		s := dns.BuildRedirectClient(p)
		if s == nil {
			return nil, fmt.Errorf("服务商 %s 暂不支持重定向规则", p.Type)
		}
		return s, nil
	})

	// 落地域名那条解析记录归 DDNS 管，同样反过来注入
	stun.RegisterLandingRecordEnsurer(ddns.Runtime.EnsureLandingRecord)
	stun.RegisterLandingRecordReleaser(ddns.Runtime.ReleaseLandingRecord)

	// MC 入口的 SRV 记录：服务商凭证在 DDNS 那边，和重定向同一个路子
	stun.RegisterSRVSyncerFactory(func(id uint) (dns.SRVRecordProvider, error) {
		p, ok := ddns.Runtime.FindProvider(id)
		if !ok {
			return nil, fmt.Errorf("DNS 服务商 %d 不存在，请先在 DDNS 里配置", id)
		}
		s := dns.BuildSRVClient(p)
		if s == nil {
			return nil, fmt.Errorf("服务商 %s 暂不支持 SRV 记录，目前只有 Cloudflare 可以", p.Type)
		}
		return s, nil
	})

	// 界面上要显示落地域名归哪条记录管：状态在 DDNS 那边，翻译成 stun 认识的样子
	stun.RegisterLandingRecordInspector(func(host string) (stun.LandingRecordState, bool) {
		rec, ok := ddns.Runtime.FindRecordByHost(host)
		if !ok {
			return stun.LandingRecordState{}, false
		}
		return stun.LandingRecordState{
			Managed: true,
			Name:    rec.Name,
			LastIP:  rec.LastIP,
			Status:  string(rec.LastStatus),
			Message: rec.LastMessage,
			At:      rec.LastSyncAt,
		}, true
	})
}

package routers

import (
	"io/fs"
	"net/http"
	_ "net/http/pprof" // 加下划线，只要副作用（自动注册路由）
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Run(webFS fs.FS) {
	// 单独起 pprof，只在排查问题时用
	// go func() {
	// 	logrus.Info("pprof 运行在：0.0.0.0:3334")
	// 	http.ListenAndServe("0.0.0.0:3334", nil)
	// }()

	gin.SetMode("release")
	// 不用 gin.Default()：它的访问日志会把 URL 查询串原样记下来，
	// 而桌面版首屏正是用 ?desktop_secret=... 传免登录凭证的，那样会把它写进日志文件。
	// 换成自定义 formatter 做脱敏，格式与 gin 默认一致（见 logger.go）。
	r := gin.New()
	r.Use(gin.LoggerWithFormatter(ginLogFormatter), gin.Recovery())
	r.RedirectTrailingSlash = false

	// 不信任任何前置代理：gin 默认信任全部代理，会让 c.ClientIP() 读取
	// X-Forwarded-For / X-Real-IP，攻击者可借伪造头冒充回环地址绕过来源校验。
	// 设为 nil 后 ClientIP() 直接返回 TCP 对端地址，与 RemoteAddr 一致。
	// 本程序的反代是出站转发，不依赖入站 XFF 解析，故无副作用。
	if err := r.SetTrustedProxies(nil); err != nil {
		logrus.Fatal("设置可信代理失败：", err)
	}

	// API 路由
	g := r.Group("api")

	// 公开接口与受保护接口两个路由组交给组件自己挂：
	// 每个 routers/<x>_routers.go 在 init() 里把自己贡献给组件注册表，
	// 挂载顺序跟着组件启动顺序走。
	//
	// 这里**不再列任何业务接口清单**。改造前是 8 个 XxxRouters(...) 调用，
	// 加一个功能要记得回来加一行；现在是「加一个 routers/<x>_routers.go」，
	// 漏加也不会静默失效 —— 注册表发现有人贡献了路由却没有同名组件会直接
	// 终止启动（见 component.MountRoutes）。
	protected := g.Group("", middleware.AuthMiddleware)

	groups := component.RouterGroups{Public: g, Protected: protected}
	mounted, err := component.MountRoutes(groups)
	if err != nil {
		// 到这一步说明代码里的组件名和路由贡献对不上，属于开发期错误。
		// 继续跑下去的表现是「整套接口 404 而日志毫无线索」，不如直接不启动。
		logrus.Fatal("挂载组件路由失败：", err)
	}
	logrus.Infof("已挂载 %d 个组件的接口：%v", len(mounted), mounted)

	// 用户上传的图标静态目录
	r.Static("/data/icon", "data/icon")
	// 用户上传的自定义壁纸
	r.Static("/data/wallpaper", "data/wallpaper")

	// 服务索引显式入口：/go/{服务名}/... → 307 到该服务当前的洞
	PortalRouters(r)

	// 剥掉 路径 前缀
	adminFS, _ := fs.Sub(webFS, "web/admin/dist")
	homeFS, _ := fs.Sub(webFS, "web/home/dist")

	// 所有非 API 请求：先找静态文件，找不到就返回 index.html（Vue Router 兜底）
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}

		if path == "/catflap" {
			c.Redirect(http.StatusMovedPermanently, "/catflap/")
			return
		}

		if strings.HasPrefix(path, "/catflap/") {
			filePath := strings.TrimPrefix(path, "/catflap/")
			if filePath != "" {
				if _, err := fs.Stat(adminFS, filePath); err == nil {
					c.FileFromFS(filePath, http.FS(adminFS))
					return
				}
			}

			data, _ := fs.ReadFile(adminFS, "index.html")
			c.Data(200, "text/html; charset=utf-8", data)
			return
		}

		filePath := strings.TrimPrefix(path, "/")
		if filePath != "" {
			if _, err := fs.Stat(homeFS, filePath); err == nil {
				c.FileFromFS(filePath, http.FS(homeFS))
				return
			}
		}

		// 裸路径服务索引：catflap.example.com/fw → 307 到 fw 当前的洞。
		// 必须排在静态文件命中之后——否则一个叫 favicon.ico 的服务名
		// 会把真实静态资源顶掉；没有同名服务则继续走前端 SPA 兜底。
		if TryPortal(c) {
			return
		}

		data, _ := fs.ReadFile(homeFS, "index.html")
		c.Data(200, "text/html; charset=utf-8", data)
	})

	logrus.Info("后端运行在：" + core.ListenAddr())

	srv := &http.Server{
		Addr:        core.ListenAddr(),
		Handler:     r,
		IdleTimeout: 60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		logrus.Fatal("启动失败：", err)
	}
}

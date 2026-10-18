package routers

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Run(webFS fs.FS) {
	// 这里**曾经** import 了 `_ "net/http/pprof"`，现在刻意去掉了。
	//
	// 那个 import 的副作用是把 /debug/pprof/* 注册到 http.DefaultServeMux 上。
	// 当前它打不到（下面用的是 gin 引擎，不是 DefaultServeMux），
	// 但留着是个地雷：谁想加个内部端口顺手写一句
	// `http.ListenAndServe(":3334", nil)`（nil = 用 DefaultServeMux），
	// 堆内存快照和协程栈就一起对外了 —— 而堆里可能有 DNS Token 和私钥。
	//
	// 要排查性能问题时临时加回来：补上 import + 单独起一个只监听 127.0.0.1 的
	// net/http.Server，用完立刻删掉，别留在发布版里。

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

	/**
	 * 这里**曾经**挂着两个静态目录（/data/icon、/data/wallpaper），是给导航页用的 ——
	 * 访客不登录也要能看到图标。
	 *
	 * 导航页整块删掉之后，后台前端根本不引用这两个路径（grep 过），
	 * 留着就是**把用户上传的文件对匿名访客开放**，没有任何收益。
	 * 将来要显示请走接口并鉴权，别再直接 Static。
	 */

	// 服务索引显式入口：/go/{服务名}/... → 307 到该服务当前的洞
	PortalRouters(r)

	// 剥掉 路径 前缀
	adminFS, _ := fs.Sub(webFS, "web/admin/dist")

	// 所有非 API 请求：先找静态文件，找不到就返回 index.html（Vue Router 兜底）
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}

		/**
		 * 管理后台直接挂在**根路径**。
		 *
		 * 以前后台在 /catflap/ 下、根路径留给导航页。
		 * 导航页砍掉之后没有第二个前端了，再保留子路径就是纯粹的多打一段字 ——
		 * 访问 http://IP:3791/ 直接进后台。
		 *
		 * 静态资源优先：/assets/xxx.js 这类能命中就按文件返回，
		 * 命中不了才回退 index.html（前端 SPA 路由靠这个）。
		 */
		filePath := strings.TrimPrefix(path, "/")
		if filePath != "" {
			if _, err := fs.Stat(adminFS, filePath); err == nil {
				c.FileFromFS(filePath, http.FS(adminFS))
				return
			}
		}

		// 裸路径服务索引：catflap.example.com/fw → 307 到 fw 当前的洞。
		// 必须排在静态文件命中之后——否则一个叫 favicon.ico 的服务名
		// 会把真实静态资源顶掉；没有同名服务则继续走前端 SPA 兜底。
		if TryPortal(c) {
			return
		}

		// 兜底：交给管理后台的 index.html（前端 SPA 路由靠这个）
		data, _ := fs.ReadFile(adminFS, "index.html")
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

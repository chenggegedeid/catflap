// Package update 后台一键更新：定时查 GitHub 最新 Release，用户点了才下载、换二进制、重启。
//
// 不做静默自动更新：重启会把所有打洞断几秒，出了问题用户也不知道是升级弄的。
package update

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"golang.org/x/net/proxy"

	"github.com/sirupsen/logrus"
)

// ReleasePage 给用户手动下载的地方
// 本仓库为 ZluxYao/Catflap（GPL-3.0-or-later）的修改版，更新源跟随本 fork
const ReleasePage = "https://github.com/chenggegedeid/catflap/releases/latest"

// latestAPI 查最新 Release 的接口。是 var 只为测试换成 httptest 地址
var latestAPI = "https://api.github.com/repos/chenggegedeid/catflap/releases/latest"

// checkInterval 后台多久查一次。GitHub 未登录接口每小时 60 次，一天一次远远够
const checkInterval = 24 * time.Hour

// ConfigPath 更新模块配置文件路径
const ConfigPath = "config/updateConfig.json"

// 下载源三选一
const (
	SourceBuiltin  = ""         // 内置：几个加速镜像轮着试，最后直连。零值，老用户默认就是它
	SourceOfficial = "official" // 只直连 GitHub，开着代理或者在海外用
	SourceCustom   = "custom"   // 只用用户自己填的前缀，内置的都失效了也有路走
)

// Config 更新模块配置
type Config struct {
	Source string `json:"source"` // SourceBuiltin / SourceOfficial / SourceCustom
	Custom string `json:"custom"` // 自定义加速前缀，如 https://ghproxy.example.com/

	// Proxy 下载更新时走的代理，如 http://127.0.0.1:7890、socks5://127.0.0.1:1080。
	// 留空表示「听环境变量的」（HTTP_PROXY / HTTPS_PROXY），也就是和以前一样。
	//
	// 为什么要在界面上配而不全靠环境变量：Catflap 多半是 systemd / 服务方式
	// 拉起来的，那种进程的**环境变量是干净的** —— 用户在 shell 里 export 了
	// 代理，服务重启后根本读不到。要有稳定的代理就得自己存一份。
	Proxy string `json:"proxy,omitempty"`
}

// builtinDownload 内置模式下载时依次试的前缀，拼在 GitHub 下载地址前面，空串是直连。
// 国内直连 GitHub 经常慢到下不完，所以加速在前、直连垫底。
// 都是第三方公益服务，随时可能失效，所以放了好几个，挂了还有「自定义」兜底。
// 它们只管搬运文件，动不了校验基准：sha256 摘要来自 builtinAPI（只直连 GitHub 原站），
// 哪个镜像改了文件都过不了校验。
var builtinDownload = []string{
	"https://ghfast.top/",
	"https://gh-proxy.com/",
	"https://gh.llkk.cc/",
	"",
}

// builtinAPI 内置模式查最新版本时用的源。**只直连，不经任何第三方镜像**（决策 D3）。
//
// 为什么这里不能省：release 里每个资产的 sha256 摘要（asset.Digest）就是从这个接口取回的，
// 下载完成后的校验也拿它当基准。换言之「摘要的可信度 = 接口的可信度」。
// 一旦 API 经第三方镜像转发，镜像方可以同时改掉二进制和它的摘要，
// 校验就变成自己跟自己比，形同虚设。下载走镜像是安全的（文件被改会被直连拿到的摘要挡下），
// 但摘要本身必须来自 GitHub 原站。
//
// 代价：国内直连 api.github.com 不通时查更新会失败。用户可挂系统代理，或改用「自定义」源
// 明确指定自己的镜像——那是知情选择，风险自负，不放在默认路径里。
var builtinAPI = []string{""}

// sources 按配置给出查版本和下载各自依次要试的前缀
func (c Config) sources() (api, download []string) {
	switch c.Source {
	case SourceOfficial:
		return []string{""}, []string{""}
	case SourceCustom:
		// 自定义的镜像不一定转发 API，查版本失败了再直连试一次，只是几 KB 的事
		p := normalizePrefix(c.Custom)
		return []string{p, ""}, []string{p}
	}
	return builtinAPI, builtinDownload
}

// normalizePrefix 补上结尾的 /，免得用户少写一个就拼成 https://ghproxy.example.comhttps://...
func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

/*
	   updateHTTPClient 更新专用的 HTTP 客户端。

	   # 为什么不用 http.DefaultClient

	   DefaultClient 用的是 DefaultTransport，它只认**环境变量**（HTTP_PROXY /
	   HTTPS_PROXY）。而 Catflap 多半以服务方式运行，那种进程的环境变量是干净的，
	   用户在 shell 里设的代理读不到 —— 于是"明明有代理、下载还是慢"。
	   所以要在配置里存一份，界面上填。

	   # 优先级

		界面填的 > 环境变量 > 直连

	   留空时退化成 ProxyFromEnvironment，老用户靠环境变量走代理的行为**完全不变**。

	   # 为什么只给更新用

	   这个代理只服务「查版本 + 下载二进制」这两件事，不影响反代与穿透：
	   那两边的目标都是内网地址，走代理纯属灾难（proxy 模块的 Transport 里
	   显式写了 Proxy: nil）。更新访问的是 GitHub，**恰恰是最需要代理的地方**。
*/
func updateHTTPClient(cfg Config) *http.Client {
	dial := (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	// socks5 不走 Proxy 字段（net/http 不认这个 scheme），改在拨号层做
	if d, ok := socksDialer(cfg.Proxy); ok {
		dial = d
	}

	transport := &http.Transport{
		Proxy: proxyFuncFor(cfg.Proxy),
		// 下面几项照抄 http.DefaultTransport：更新是几个到几十 MB 的下载，
		// 默认值已经合适，没必要调
		DialContext:           dial,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Transport: transport}
}

// proxyFuncFor 按配置的代理地址给出 http.Transport 用的 Proxy 函数。
//
// 支持的写法（scheme 可省略，省了默认 http）：
//   - http://127.0.0.1:7890  —— Clash / V2Ray 这类最常见的 HTTP 代理
//   - socks5://127.0.0.1:1080
//   - 留空 → 交给环境变量
//
// socks5 走 DialContext 而不是 Proxy 字段：net/http 的 Proxy 只认
// http/https 两种 scheme，socks5 得自己在拨号层做。
func proxyFuncFor(p string) func(*http.Request) (*url.URL, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return http.ProxyFromEnvironment
	}

	// 用户多半只会填 127.0.0.1:7890，补上 scheme 免得解析出个奇怪的 URL
	if !strings.Contains(p, "://") {
		p = "http://" + p
	}

	u, err := url.Parse(p)
	if err != nil || u.Host == "" {
		// 填错了不该让更新直接不能用：退回去听环境变量的，并在日志里说一声
		logrus.Warnf("[update] 代理地址 %q 不合法，本次更新改为直连（或按环境变量走）", p)
		return http.ProxyFromEnvironment
	}

	switch u.Scheme {
	case "socks5", "socks5h":
		// 这两个 scheme net/http 的 Proxy 不认，交给拨号层
		return nil // 见下面的 DialContext 覆盖
	default:
		return http.ProxyURL(u)
	}
}

// socksDialer 配置的代理是 socks5 时，替换掉 Transport 的拨号函数。
// 不是 socks5 就返回 nil，保持默认。
func socksDialer(p string) (func(ctx context.Context, network, addr string) (net.Conn, error), bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil, false
	}
	if !strings.Contains(p, "://") {
		return nil, false // 没写 scheme 就默认是 http，不走 socks
	}
	u, err := url.Parse(p)
	if err != nil || (u.Scheme != "socks5" && u.Scheme != "socks5h") {
		return nil, false
	}

	// 带认证的 socks5：user:pass@host:port
	var auth *proxy.Auth
	if u.User != nil {
		auth = &proxy.Auth{User: u.User.Username()}
		if pw, ok := u.User.Password(); ok {
			auth.Password = pw
		}
	}
	dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		logrus.Warnf("[update] socks5 代理 %q 不可用，改为直连：%v", p, err)
		return nil, false
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// x/net/proxy 的 Dialer 不吃 context，这里用 Dial 即可。
		// 超时由 Transport 的 ResponseHeaderTimeout / stallTimeout 兜住。
		return dialer.Dial(network, addr)
	}, true
}

// stallTimeout 下载多久没进展就放弃这个源、换下一个。镜像挂了常常是连上了但不给数据。
// 是 var 只为测试调短
var stallTimeout = 20 * time.Second

// maxUploadSize 手动上传的程序最大多少。现在各平台都在 30MB 以内，留足余量
const maxUploadSize = 200 << 20

// startedAt 本进程启动时间。前端靠它变没变判断重启完成——
// 手动上传可能装回同一个版本，看版本号判断不了
var startedAt = time.Now()

// Job 一次更新的进度
type Job struct {
	State  string `json:"state"`  // "" 空闲 / "downloading" / "restarting" / "failed"
	Source string `json:"source"` // 正在从哪儿下，给界面显示
	Done   int64  `json:"done"`   // 已下载字节
	Total  int64  `json:"total"`  // 文件总字节
	Error  string `json:"error"`
}

// Status 界面上要的全部
type Status struct {
	Current    string     `json:"current"`
	Latest     string     `json:"latest"`
	HasUpdate  bool       `json:"hasUpdate"`
	Notes      string     `json:"notes"`
	ReleaseURL string     `json:"releaseUrl"`
	CheckedAt  *time.Time `json:"checkedAt,omitempty"`
	CheckError string     `json:"checkError"`
	// ManualReason 不能一键更新的原因（没有对应平台的文件、飞牛应用中心装的……），
	// 空串表示可以。有新版本时界面照样提示，让用户去 Releases 页下载
	ManualReason string `json:"manualReason"`
	// UploadReason 不能手动上传更新的原因，空串表示可以
	UploadReason string `json:"uploadReason"`
	// Asset 本机对应的发布文件名，提示用户该传哪个；没有对应的是空串
	Asset     string    `json:"asset"`
	Config    Config    `json:"config"`
	StartedAt time.Time `json:"startedAt"`
	Job       Job       `json:"job"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Body    string         `json:"body"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Digest      string `json:"digest"` // "sha256:..."，GitHub 2025 年起给每个资产都算
	DownloadURL string `json:"browser_download_url"`
}

// UpdateRuntime 更新模块运行时
type UpdateRuntime struct {
	mu        sync.RWMutex
	config    Config
	version   string
	latest    *release
	checkedAt *time.Time
	checkErr  string
	job       Job
	done      atomic.Int64 // 下载进度单独放，下载循环里不用抢锁
}

// Runtime 更新模块全局运行时
var Runtime = &UpdateRuntime{}

// desktopQuit 桌面版注册的退出函数，见 RegisterDesktop
var desktopQuit func()

// configSnapshot 取一份配置副本。
//
// 下载是几十 MB 的长活儿，全程拿锁不合适；但也不能直接读 r.config
// （正在下载时用户在界面上改配置就会 data race）。
// 开头读一次快照就够：代理这种设置本来也不需要中途生效。
func (r *UpdateRuntime) configSnapshot() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// RegisterDesktop 桌面版启动时调用。一来认出自己是桌面版，下载 catflap-desktop.exe；
// 二来重启时要走 Wails 的 Quit，托盘图标才会收掉，直接 os.Exit 会在任务栏留个死图标。
func RegisterDesktop(quit func()) {
	desktopQuit = quit
}

// InitUpdate 读配置、起后台定时检查。version 由 main 包传进来——
// 版本号在 api/system_api 里，modules 不能反过来 import api
func InitUpdate(version string) error {
	cfg, err := readConfig()
	if err != nil {
		return fmt.Errorf("读取更新配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.config = cfg
	Runtime.version = version
	Runtime.mu.Unlock()
	removeOldExecutable()

	go func() {
		// 刚启动网络未必通（开机自启时网卡还没拿到地址），晚一点再查
		time.Sleep(30 * time.Second)
		for {
			// 关掉「自动更新」之后就别再往外发请求了。
			//
			// 这个循环是永久的，开关翻位不会停它。不挡这一下的话，界面上写着
			// 已关闭，后台仍然每隔 checkInterval 请求一次 GitHub —— 而 GitHub 对
			// 未认证请求有每小时 60 次的限流，一个关掉了的功能照样能把额度吃掉。
			if !Enabled() {
				time.Sleep(checkInterval)
				continue
			}
			if err := Runtime.Check(context.Background()); err != nil {
				logrus.Warnf("检查更新失败：%v", err)
			}
			time.Sleep(checkInterval)
		}
	}()
	return nil
}

// Enabled 报告自动更新功能当前是否开启。
//
// 走 component.IsEnabled 而不是另存一份状态：开关的真相源只有
// config/components.json 一个。
func Enabled() bool { return component.IsEnabled(Name) }

func readConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		cfg := Config{}
		if err := utilsFile.EnsurePrivateDir(path.Dir(ConfigPath)); err != nil {
			return cfg, err
		}
		return cfg, utilsFile.WriteJsonFile(ConfigPath, cfg)
	}
	return utilsFile.ReadJsonFile[Config](ConfigPath)
}

// SetConfig 保存下载源设置
func (r *UpdateRuntime) SetConfig(cfg Config) error {
	cfg.Custom = normalizePrefix(cfg.Custom)
	switch cfg.Source {
	case SourceBuiltin, SourceOfficial:
	case SourceCustom:
		if !strings.HasPrefix(cfg.Custom, "http://") && !strings.HasPrefix(cfg.Custom, "https://") {
			return errors.New("自定义下载源要填一个 http:// 或 https:// 开头的地址")
		}
	default:
		return errors.New("下载源选项不对")
	}
	r.mu.Lock()
	r.config = cfg
	r.mu.Unlock()
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		return fmt.Errorf("保存更新配置失败: %w", err)
	}
	return nil
}

// Status 当前状态快照
func (r *UpdateRuntime) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s := Status{
		Current:    r.version,
		ReleaseURL: ReleasePage,
		CheckedAt:  r.checkedAt,
		CheckError: r.checkErr,
		Asset:      assetName(),
		Config:     r.config,
		StartedAt:  startedAt,
		Job:        r.job,
	}
	s.Job.Done = r.done.Load()
	if err := selfReplaceBlocked(); err != nil {
		s.UploadReason = err.Error()
	}
	if r.latest != nil {
		s.Latest = releaseVersion(r.latest.TagName)
		s.HasUpdate = newerThan(s.Latest, r.version)
		s.Notes = r.latest.Body
		if r.latest.HTMLURL != "" {
			s.ReleaseURL = r.latest.HTMLURL
		}
		if _, err := pickAsset(r.latest); err != nil {
			s.ManualReason = err.Error()
		}
	}
	return s
}

// Check 立刻查一次最新 Release
func (r *UpdateRuntime) Check(ctx context.Context) error {
	r.mu.RLock()
	apiSources, _ := r.config.sources()
	r.mu.RUnlock()
	rel, err := fetchLatest(ctx, apiSources, r.configSnapshot())
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkedAt = &now
	if err != nil {
		r.checkErr = err.Error()
		return err
	}
	r.checkErr = ""
	r.latest = rel
	return nil
}

// fetchLatest 依次从各个源查最新 Release，直连不通（国内常见）或被限流时换下一个
func fetchLatest(ctx context.Context, sources []string, cfg Config) (*release, error) {
	var firstErr error
	for _, src := range sources {
		rel, err := fetchLatestFrom(ctx, src+latestAPI, cfg)
		if err == nil {
			return rel, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		logrus.Debugf("从 %s 查更新失败：%v", sourceName(src), err)
	}
	return nil, firstErr
}

func fetchLatestFrom(ctx context.Context, url string, cfg Config) (*release, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// 走配置了代理的客户端（见 updateHTTPClient 的说明）
	resp, err := updateHTTPClient(cfg).Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上 GitHub，检查一下网络或代理设置：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("GitHub 限制了这个 IP 的查询次数，过一小时再试")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("解析 GitHub 返回失败：%w", err)
	}
	if rel.TagName == "" {
		return nil, errors.New("GitHub 返回的内容不对") // 镜像出错时可能回一个别的 JSON
	}
	return &rel, nil
}

// sourceName 给日志和界面看的源名字
func sourceName(prefix string) string {
	if prefix == "" {
		return "GitHub 直连"
	}
	return strings.TrimSuffix(strings.TrimPrefix(prefix, "https://"), "/")
}

// releaseVersion "Catflap-1.0.0" / "v1.0.0" → "1.0.0"
func releaseVersion(tag string) string {
	if i := strings.LastIndex(tag, "-"); i >= 0 {
		tag = tag[i+1:]
	}
	return strings.TrimPrefix(tag, "v")
}

// newerThan a 是否比 b 新，按点分数字逐段比
func newerThan(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// assetName 这份二进制对应 Release 里的哪个文件，没有就返回空串
func assetName() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		if desktopQuit != nil {
			return "catflap-desktop.exe"
		}
	case "linux/amd64":
		return "catflap"
	case "linux/arm64":
		return "catflap-linux-arm64"
	case "linux/arm":
		return "catflap-linux-armv7"
	case "linux/mipsle":
		return "catflap-linux-mipsle"
	}
	return ""
}

// selfReplaceBlocked 这种装法能不能自己换程序、自己重启，不能就返回给用户看的原因。
// 一键更新和手动上传都要先过这一关
func selfReplaceBlocked() error {
	// 飞牛的应用由应用中心管着，自己换掉二进制，应用中心记的版本就对不上了
	if os.Getenv("TRIM_APPNAME") != "" {
		return errors.New("飞牛 NAS 上请下载新的 fpk，在应用中心里覆盖安装")
	}
	// macOS 桌面版是 .app 包，换里面的二进制会破坏签名
	if runtime.GOOS == "darwin" && desktopQuit != nil {
		return errors.New("macOS 桌面版请手动替换")
	}
	return nil
}

// pickAsset 挑出这台机器该下的文件；不能一键更新时返回给用户看的原因
func pickAsset(rel *release) (releaseAsset, error) {
	if err := selfReplaceBlocked(); err != nil {
		return releaseAsset{}, err
	}
	name := assetName()
	if name == "" {
		return releaseAsset{}, errors.New("Release 里没有这个平台的文件，请下载后手动上传")
	}
	for _, a := range rel.Assets {
		if a.Name == name {
			if !strings.HasPrefix(a.Digest, "sha256:") {
				return releaseAsset{}, errors.New("这个版本的发布文件没有校验值，为安全起见请手动下载")
			}
			return a, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("新版本里没有 %s，请手动下载", name)
}

// Install 开始下载更新，立刻返回；进度从 Status 里看。下载校验完会自己重启
func (r *UpdateRuntime) Install() error {
	r.mu.Lock()
	if r.job.State == "downloading" || r.job.State == "restarting" {
		r.mu.Unlock()
		return errors.New("已经在更新了")
	}
	if r.latest == nil || !newerThan(releaseVersion(r.latest.TagName), r.version) {
		r.mu.Unlock()
		return errors.New("已经是最新版本")
	}
	asset, err := pickAsset(r.latest)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	_, sources := r.config.sources()
	r.job = Job{State: "downloading", Total: asset.Size}
	r.done.Store(0)
	r.mu.Unlock()

	go func() {
		err := r.download(asset, sources)
		r.mu.Lock()
		if err != nil {
			r.job.State, r.job.Error = "failed", err.Error()
		} else {
			r.job.State = "restarting"
		}
		r.mu.Unlock()
		if err != nil {
			logrus.Errorf("更新失败：%v", err)
			return
		}

		logrus.Infof("已换成 %s，正在重启", asset.Name)
		r.restart()
	}()
	return nil
}

// InstallUpload 手动上传的程序：先确认是这台机器能跑的 Catflap，再换上、重启。
// 不比版本新旧，回退到老版本也走这里
func (r *UpdateRuntime) InstallUpload(src io.Reader) error {
	if err := selfReplaceBlocked(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.job.State == "downloading" || r.job.State == "restarting" {
		r.mu.Unlock()
		return errors.New("已经在更新了")
	}
	r.job = Job{State: "downloading"}
	r.mu.Unlock()

	err := installUpload(src)
	r.mu.Lock()
	if err != nil {
		r.job = Job{} // 文件不对是用户的事，不算一次失败的更新，界面上直接报错就行
	} else {
		r.job.State = "restarting"
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}

	logrus.Info("已换成手动上传的程序，正在重启")
	go r.restart()
	return nil
}

func installUpload(src io.Reader) error {
	exe, err := executablePath()
	if err != nil {
		return fmt.Errorf("找不到程序自身的位置：%w", err)
	}
	tmp := exe + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("程序所在目录 %s 不能写，请手动替换：%w", filepath.Dir(exe), err)
	}
	defer os.Remove(tmp)
	_, err = io.Copy(f, io.LimitReader(src, maxUploadSize))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("保存上传的文件失败：%w", err)
	}
	if err := checkBinary(tmp); err != nil {
		return err
	}
	return replaceExecutable(exe, tmp)
}

// checkBinary 读 Go 编进程序里的构建信息，挡住传错的文件：
// 不是 Catflap、系统或架构不对、桌面版和命令行版传反了。换上去起不来，就只能上机器救了
//
// ⚠️ 能力边界：这**不是**完整性校验。buildinfo 段与代码段分离，
// 一个真 Catflap 二进制被改掉代码区的字节后，buildinfo 依然可读、Path 依然是本项目，
// 本函数会放行。手动上传没有可信的 sha256 基准可对比（文件来自用户本地），
// 所以这条路只能防「装错文件把服务搞挂」，防不了「有人塞了个改过的 Catflap」。
// 请从官方 Releases 页下载后再上传。下载路径由 downloadFrom 的 sha256 校验把关，
// 摘要来自只直连 GitHub 原站的 builtinAPI，那条路是可信的。
func checkBinary(path string) error {
	info, err := buildinfo.ReadFile(path)
	// 用常量而不是字面量：module 路径一改，这里的字面量就会漏改，
	// 而漏改的表现是「一键更新拒绝所有合法文件」—— 不是一眼能看出来的故障。
	if err != nil || info.Path != core.ModulePath {
		return errors.New("这不是 Catflap 程序，请上传 Release 里下载的文件")
	}
	var goos, goarch, tags string
	for _, s := range info.Settings {
		switch s.Key {
		case "GOOS":
			goos = s.Value
		case "GOARCH":
			goarch = s.Value
		case "-tags":
			tags = s.Value
		}
	}
	if goos != runtime.GOOS || goarch != runtime.GOARCH {
		return fmt.Errorf("这个文件是 %s/%s 的，这台机器是 %s/%s，请换对应的文件", goos, goarch, runtime.GOOS, runtime.GOARCH)
	}
	isDesktop := false
	for _, t := range strings.FieldsFunc(tags, func(r rune) bool { return r == ',' || r == ' ' }) {
		if t == "desktop" {
			isDesktop = true
		}
	}
	if isDesktop != (desktopQuit != nil) {
		if isDesktop {
			return errors.New("这是桌面版，现在跑的是命令行版，请换命令行版的文件")
		}
		return errors.New("这是命令行版，现在跑的是桌面版，请换 catflap-desktop.exe")
	}
	return nil
}

// restart 新程序已经换好，重启成它
func (r *UpdateRuntime) restart() {
	// 留一点时间让前端轮询到「正在重启」、拿到上传接口的返回，不然它只会看到连接断了
	time.Sleep(time.Second)
	if err := restart(); err != nil {
		// 文件已经换好，只是没能自己拉起来：下次手动启动就是新版本
		logrus.Errorf("新版本已就位，但自动重启失败，请手动重启：%v", err)
		r.mu.Lock()
		r.job.State, r.job.Error = "failed", "新版本已经换好，但没能自动重启，请手动重启 Catflap："+err.Error()
		r.mu.Unlock()
	}
}

// errDirNotWritable 程序目录写不进去，换哪个源都没用，不用往下试了
var errDirNotWritable = errors.New("程序目录不能写")

// download 依次从各个源下载到可执行文件旁边，校验 sha256 通过就换掉正在运行的那个
func (r *UpdateRuntime) download(asset releaseAsset, sources []string) error {
	exe, err := executablePath()
	if err != nil {
		return fmt.Errorf("找不到程序自身的位置：%w", err)
	}
	tmp := exe + ".new"
	defer os.Remove(tmp) // 换成功后它已经不在了，Remove 无害

	var errs []string
	for _, src := range sources {
		r.mu.Lock()
		r.job.Source = sourceName(src)
		r.mu.Unlock()
		r.done.Store(0)

		err := r.downloadFrom(src+asset.DownloadURL, tmp, asset)
		if err == nil {
			logrus.Infof("从 %s 下载 %s 完成", sourceName(src), asset.Name)
			return replaceExecutable(exe, tmp)
		}
		if errors.Is(err, errDirNotWritable) {
			return fmt.Errorf("程序所在目录 %s 不能写，请手动替换：%w", filepath.Dir(exe), err)
		}
		logrus.Warnf("从 %s 下载失败，换下一个：%v", sourceName(src), err)
		errs = append(errs, sourceName(src)+" "+err.Error())
	}
	return errors.New("下载失败，换个下载源再试，或者去 Releases 页下载后手动上传（" + strings.Join(errs, "；") + "）")
}

// downloadFrom 从一个地址下载到 tmp 并校验。stallTimeout 内一个字节都没多就放弃
func (r *UpdateRuntime) downloadFrom(url, tmp string, asset releaseAsset) error {
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("%w：%v", errDirNotWritable, err)
	}
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		last := r.done.Load()
		t := time.NewTicker(stallTimeout)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := r.done.Load()
				if now == last {
					cancel()
					return
				}
				last = now
			}
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// 走配置了代理的客户端（见 updateHTTPClient 的说明）
	resp, err := updateHTTPClient(r.configSnapshot()).Do(req)
	if err != nil {
		return fmt.Errorf("连不上：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	h := sha256.New()
	// 多读 1 字节：镜像塞了更大的东西进来也能发现，不会一直读下去
	if _, err := io.Copy(io.MultiWriter(f, h, progressWriter{&r.done}), io.LimitReader(resp.Body, asset.Size+1)); err != nil {
		return fmt.Errorf("下载中断：%w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if "sha256:"+hex.EncodeToString(h.Sum(nil)) != asset.Digest {
		return errors.New("文件校验不对，可能被镜像改过或者没下完整")
	}
	return nil
}

type progressWriter struct{ n *atomic.Int64 }

func (p progressWriter) Write(b []byte) (int, error) {
	p.n.Add(int64(len(b)))
	return len(b), nil
}

// executablePath 程序自身的路径，只在第一次调用时算。
// 必须在换文件之前定下来：Linux 上旧文件被 rename 顶掉之后，/proc/self/exe
// 读出来是「/opt/catflap/catflap (deleted)」，重启时就找不到新文件了
var executablePath = sync.OnceValues(func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
})

// quit 保存配置退出，给 Windows 的「退出后再拉起新进程」用
func quit() {
	if desktopQuit != nil {
		desktopQuit() // Wails 的 OnShutdown 里会调 core.RunShutdown
		return
	}
	core.RunShutdown()
	os.Exit(0)
}

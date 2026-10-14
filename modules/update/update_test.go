package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewerThan(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"补丁号更大", "0.7.1", "0.7.0", true},
		{"相同版本", "0.7.0", "0.7.0", false},
		{"旧版本", "0.6.5", "0.7.0", false},
		{"两位数不按字符串比", "0.10.0", "0.9.0", true},
		{"段数不同", "0.7", "0.6.5", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := newerThan(c.a, c.b); got != c.want {
				t.Fatalf("newerThan(%q, %q) = %v，期望 %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestReleaseVersion(t *testing.T) {
	for tag, want := range map[string]string{
		"Catflap-1.0.0": "1.0.0",
		"v1.0.0":        "1.0.0",
		"1.1.0":         "1.1.0",
	} {
		if got := releaseVersion(tag); got != want {
			t.Fatalf("releaseVersion(%q) = %q，期望 %q", tag, got, want)
		}
	}
}

func TestDownload(t *testing.T) {
	newBin := []byte("new catflap binary")
	sum := sha256.Sum256(newBin)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	// 每个源一种表现：ok 正常给文件、tampered 给改过的、down 回 502、stall 连上了不给数据
	cases := []struct {
		name       string
		sources    []string
		wantErr    string
		wantFile   string
		wantSource string
	}{
		{name: "第一个源就成功", sources: []string{"ok"}, wantFile: string(newBin), wantSource: "/ok/"},
		{name: "镜像挂了换下一个", sources: []string{"down", "ok"}, wantFile: string(newBin), wantSource: "/ok/"},
		{name: "镜像改了文件换下一个", sources: []string{"tampered", "ok"}, wantFile: string(newBin), wantSource: "/ok/"},
		{name: "镜像卡住不给数据换下一个", sources: []string{"stall", "ok"}, wantFile: string(newBin), wantSource: "/ok/"},
		{name: "全都失败不换", sources: []string{"down", "tampered"}, wantErr: "下载失败", wantFile: "old"},
		// 长度一致、只翻转一字节：只能靠 sha256 发现，长度对不上那种检查挡不住
		{name: "镜像只改一字节换下一个", sources: []string{"flipped", "ok"}, wantFile: string(newBin), wantSource: "/ok/"},
		{name: "只改一字节且无备源必须拒绝", sources: []string{"flipped"}, wantErr: "文件校验不对", wantFile: "old"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			release := make(chan struct{})
			var lastOK string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mode := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
				// 镜像的用法是把完整的 GitHub 地址拼在前缀后面
				if !strings.HasSuffix(r.URL.Path, "/https://github.com/x/catflap") {
					t.Errorf("镜像请求路径不对：%s", r.URL.Path)
				}
				switch mode {
				case "ok":
					lastOK = "/ok/"
					_, _ = w.Write(newBin)
				case "tampered":
					_, _ = w.Write([]byte("tampered catflap bin"))
				case "flipped":
					// 与真文件等长，仅末位异或 1
					b := make([]byte, len(newBin))
					copy(b, newBin)
					b[len(b)-1] ^= 0x01
					_, _ = w.Write(b)
				case "down":
					w.WriteHeader(http.StatusBadGateway)
				case "stall":
					w.(http.Flusher).Flush()
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}
			}))
			defer srv.Close()
			defer close(release)

			exe := filepath.Join(t.TempDir(), "catflap")
			if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
				t.Fatal(err)
			}
			origExe, origStall := executablePath, stallTimeout
			executablePath = func() (string, error) { return exe, nil }
			var sources []string
			for _, m := range c.sources {
				sources = append(sources, srv.URL+"/"+m+"/")
			}
			stallTimeout = 200 * time.Millisecond
			defer func() { executablePath, stallTimeout = origExe, origStall }()

			asset := releaseAsset{Name: "catflap", Size: int64(len(newBin)), Digest: digest, DownloadURL: "https://github.com/x/catflap"}
			err := (&UpdateRuntime{}).download(asset, sources)
			if c.wantErr == "" && err != nil {
				t.Fatalf("不该出错：%v", err)
			}
			if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("期望错误含 %q，实际 %v", c.wantErr, err)
			}
			if c.wantSource != "" && lastOK != c.wantSource {
				t.Fatalf("应该从 %s 下成功", c.wantSource)
			}
			got, _ := os.ReadFile(exe)
			if string(got) != c.wantFile {
				t.Fatalf("程序文件内容 = %q，期望 %q", got, c.wantFile)
			}
			if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
				t.Fatal("临时文件没清掉")
			}
		})
	}
}

func TestFetchLatestFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/limited/") {
			w.WriteHeader(http.StatusForbidden) // 直连被 GitHub 限流
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"Catflap-9.9.9"}`))
	}))
	defer srv.Close()

	origAPI := latestAPI
	latestAPI = "https://api.github.com/repos/x/releases/latest"
	defer func() { latestAPI = origAPI }()

	rel, err := fetchLatest(context.Background(), []string{srv.URL + "/limited/", srv.URL + "/mirror/"})
	if err != nil {
		t.Fatalf("第一个源被限流应换下一个：%v", err)
	}
	if releaseVersion(rel.TagName) != "9.9.9" {
		t.Fatalf("版本 = %s", rel.TagName)
	}
}

func TestCheckBinary(t *testing.T) {
	// 拿测试程序自己当样本：它也是 Go 编的，但模块路径是 linkstar 的测试包
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(t.TempDir(), "junk")
	if err := os.WriteFile(junk, []byte("not a binary"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"随便一个文件不认", junk, "不是 Catflap"},
		{"go test 编出来的不是 catflap 主程序", self, "不是 Catflap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkBinary(c.path)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("期望错误含 %q，实际 %v", c.wantErr, err)
			}
		})
	}
}

// 回归防护：内置 API 源必须只直连 GitHub 原站。
// 摘要的可信度等于接口的可信度——这里一旦混进第三方镜像，
// 攻击者可同时替换二进制与其 sha256 摘要，下载校验就完全失效了。
func TestBuiltinAPIIsDirectOnly(t *testing.T) {
	if len(builtinAPI) != 1 || builtinAPI[0] != "" {
		t.Fatalf("builtinAPI 必须只含直连（空串），实际 = %v", builtinAPI)
	}
	// 下载前缀必须是空串或 https:// 开头，防止手滑写进奇怪的东西
	for _, p := range builtinDownload {
		if p != "" && !strings.HasPrefix(p, "https://") {
			t.Fatalf("下载前缀格式不对：%q", p)
		}
	}
}

func TestConfigSources(t *testing.T) {
	cases := []struct {
		name         string
		cfg          Config
		wantAPI      []string
		wantDownload []string
	}{
		{"老配置没这一段按内置", Config{}, builtinAPI, builtinDownload},
		{"官方只直连", Config{Source: SourceOfficial}, []string{""}, []string{""}},
		{"自定义补上结尾斜杠", Config{Source: SourceCustom, Custom: "https://ghproxy.example.com"},
			[]string{"https://ghproxy.example.com/", ""}, []string{"https://ghproxy.example.com/"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api, dl := c.cfg.sources()
			if strings.Join(api, ",") != strings.Join(c.wantAPI, ",") || strings.Join(dl, ",") != strings.Join(c.wantDownload, ",") {
				t.Fatalf("sources() = %v %v，期望 %v %v", api, dl, c.wantAPI, c.wantDownload)
			}
		})
	}
}

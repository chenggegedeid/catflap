package audit

import "strings"

// 攻击类型。界面上「攻击类型」那块按它分组排行。
//
// 这是**分类**不是**判定**：我们不拦截这些东西（拦不拦由反代/AccessControl 决定），
// 只是把「谁在扫我」的几百条 4xx 归纳成人能看懂的几个桶。
type AttackKind string

const (
	AttackScan    AttackKind = "scan"    // 路径扫描：到处试目录和常见文件
	AttackSecret  AttackKind = "secret"  // 敏感文件探测：.env、.git、备份文件
	AttackAdmin   AttackKind = "admin"   // 后台爆破：wp-login、phpmyadmin、管理入口
	AttackInject  AttackKind = "inject"  // 注入尝试：带 ../ 或 SQL 关键字
	AttackOther   AttackKind = "other"   // 其他 4xx
	AttackBlocked AttackKind = "blocked" // 被拦截（403/429，来自封禁或访问控制）
)

// AttackKindLabel 给人看的名字。顺序就是界面上的展示顺序（危害从高到低）。
var attackKindOrder = []AttackKind{AttackSecret, AttackAdmin, AttackInject, AttackScan, AttackBlocked, AttackOther}

var attackKindLabels = map[AttackKind]string{
	AttackSecret:  "敏感文件探测",
	AttackAdmin:   "后台爆破",
	AttackInject:  "注入尝试",
	AttackScan:    "路径扫描",
	AttackBlocked: "被拦截",
	AttackOther:   "其他错误",
}

// Label 这个分类的中文名。
func (k AttackKind) Label() string {
	if s, ok := attackKindLabels[k]; ok {
		return s
	}
	return string(k)
}

// attackRule 一条判定规则。
//
// 写成表而不是一串 if：
//   - 加规则不用改逻辑，改一行数据；
//   - 规则能单独测（见 attack_test.go），不用为了测一条规则把整个判定函数喂满参数。
type attackRule struct {
	kind AttackKind
	// contains 路径里出现这些子串之一就算。全部小写比较
	contains []string
	// suffix 路径以这些结尾才算（".zip" 这种不能用 contains：
	// "/download/zip" 也会命中，而它是个正常路径）
	suffix []string
}

// attackRules 判定规则表。**顺序有意义**：先匹配到的赢。
//
// 危害高的排前面：一个请求如果既是「敏感文件」又像「路径扫描」，
// 应当归到更值得关注的那一类。
var attackRules = []attackRule{
	{
		kind: AttackSecret,
		contains: []string{
			"/.env", "/.git", "/.svn", "/.ds_store", "/.aws", "/.ssh",
			"/backup", "/dump", "/.sql", "/config.php.bak", "/web.config",
			"/phpinfo", "/.htpasswd", "/.htaccess", "/id_rsa", "/.bash_history",
			"/wp-config", "/database.yml", "/credentials",
		},
		suffix: []string{".sql", ".bak", ".old", ".zip", ".tar.gz", ".tgz", ".rar"},
	},
	{
		kind: AttackAdmin,
		contains: []string{
			"/wp-admin", "/wp-login", "/xmlrpc.php", "/phpmyadmin", "/pma/",
			"/admin", "/manager/html", "/cgi-bin/", "/druid/", "/actuator",
			"/solr/", "/console", "/jenkins", "/zabbix", "/grafana",
		},
	},
	{
		kind: AttackInject,
		// 只收**语法特征**，不收参数名。
		//
		// 曾经把 "cmd=" 也放进来了，结果 /device.rsp?opt=sys&cmd=__ 这种
		// 路由器漏洞扫描被归成「注入」—— 那是扫描不是注入，归错了会让
		// 「攻击类型」这块的数字看起来比实际严重。分类宁可粗，不要错。
		contains: []string{
			"../", "..%2f", "%2e%2e", "etc/passwd", "proc/self",
			"union select", "union%20select", "union+select", "select%20", "select+",
			"' or ", "or 1=1", "and 1=1", "1=1",
			"<script", "%3cscript", "javascript:", "${jndi", "base64_decode",
			"sleep(", "benchmark(", "information_schema",
		},
	},
	{
		// 路径扫描放最后：它的特征最宽，任何 404 都容易沾上
		kind: AttackScan,
		contains: []string{
			"/cgi-bin/", "/vendor/", "/setup", "/install", "/test", "/demo",
			"/shell", "/.well-known/", "/boaform/", "/hnap1", "/login.cgi",
			"/GponForm/", "/remote/", "/HNAP1", "/wls-wsat", "/console/login",
			// 家用路由器 / 摄像头漏洞的固定指纹。这类流量在国内家宽上极多，
			// 不单独列出来就会被归到「其他错误」里，看着像正常 404
			"/device.rsp", "/picsdesc.xml", "/onvif/", "/hudson", "/jmx-console",
			"/struts/", "/.ds_store", "/cvs/", "/.svn/",
		},
		// 这些后缀是漏洞扫描器在找一个"目录"或"文件是否存在"
		suffix: []string{".php~", ".php.bak", ".asp", ".aspx", ".jsp"},
	},
}

// ClassifyAttack 给一条访问记录归类。
//
// 只看**出错的请求**（4xx/5xx）：一个 200 的 /favicon.ico 不是攻击，
// 而一个 404 的 /.env 一定有人在找东西。把 200 也算进来会让「攻击类型」
// 那块被正常流量淹没。
func ClassifyAttack(r Record) AttackKind {
	if !r.isError() {
		return AttackOther
	}
	// 被封禁/被访问控制挡下来的，先归「被拦截」——
	// 这些请求本身就是「我们拦住了什么」的答案，比它请求了什么更有意义
	if r.Status == 403 || r.Status == 429 {
		return AttackBlocked
	}

	path := strings.ToLower(r.Path)
	if path == "" {
		return AttackOther
	}

	for _, rule := range attackRules {
		for _, s := range rule.contains {
			if strings.Contains(path, s) {
				return rule.kind
			}
		}
		for _, s := range rule.suffix {
			if strings.HasSuffix(path, s) {
				return rule.kind
			}
		}
	}
	return AttackOther
}

// AttackKindOrder 界面上的展示顺序。
func AttackKindOrder() []AttackKind {
	out := make([]AttackKind, len(attackKindOrder))
	copy(out, attackKindOrder)
	return out
}

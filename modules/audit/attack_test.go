package audit

import "testing"

// TestClassifyAttack 规则表的基本判定。
//
// 每条规则至少有一个正例：加规则时如果写错了（比如把 contains 写成 suffix），
// 这里会红。规则表是数据，数据错了不会编译失败，只能靠这类用例盯着。
func TestClassifyAttack(t *testing.T) {
	cases := []struct {
		path   string
		status int
		want   AttackKind
	}{
		// 敏感文件
		{"/.env", 404, AttackSecret},
		{"/.git/config", 404, AttackSecret},
		{"/backup/db.sql", 200 - 200 + 404, AttackSecret},
		{"/wp-config.php.bak", 404, AttackSecret},
		{"/download/archive.tar.gz", 404, AttackSecret},

		// 后台爆破
		{"/wp-login.php", 404, AttackAdmin},
		{"/wp-admin/", 403, AttackBlocked}, // 403 优先归「被拦截」
		{"/phpmyadmin/index.php", 404, AttackAdmin},
		{"/admin/", 404, AttackAdmin},
		{"/actuator/env", 404, AttackAdmin},

		// 注入：只认语法特征
		{"/../../etc/passwd", 400, AttackInject},
		{"/?id=1' OR 1=1--", 500, AttackInject},
		{"/index.php?id=1+union+select+1", 404, AttackInject},
		// 带 cmd= 的路由器漏洞扫描是**扫描**不是注入，别归错
		{"/device.rsp?opt=sys&cmd=__", 404, AttackScan},

		// 路径扫描
		{"/setup.cgi", 404, AttackScan},

		// 其他
		{"/api/nonexist", 404, AttackOther},
		{"/favicon.ico", 200, AttackOther}, // 正常请求不算攻击
		{"", 404, AttackOther},
	}

	for _, c := range cases {
		got := ClassifyAttack(Record{Path: c.path, Status: c.status})
		if got != c.want {
			t.Errorf("ClassifyAttack(%q, %d) = %q，期望 %q", c.path, c.status, got, c.want)
		}
	}
}

// TestClassifyAttackIgnoresSuccess 成功请求一条都不该被归类成攻击。
//
// 这条单独拎出来：审计页的「攻击类型」是给人看「有人在打我」的，
// 把正常访问混进去就失去了意义。曾经考虑过"路径像 .env 就算"，
// 那会把用户自己排查时访问的合法路径也标红。
func TestClassifyAttackIgnoresSuccess(t *testing.T) {
	for _, p := range []string{"/.env", "/wp-admin/", "/../../etc/passwd", "/backup/db.sql"} {
		if got := ClassifyAttack(Record{Path: p, Status: 200}); got != AttackOther {
			t.Errorf("200 的 %q 被归类成 %q，应当不参与攻击分类", p, got)
		}
	}
}

// TestAttackLabelsCoverAllKinds 每个分类都要有中文名。
//
// 漏一个的话界面上会显示英文 key（"secret"），用户看不懂。
func TestAttackLabelsCoverAllKinds(t *testing.T) {
	for _, k := range AttackKindOrder() {
		if k.Label() == string(k) {
			t.Errorf("分类 %q 没有中文名", k)
		}
	}
	for k := range attackKindLabels {
		found := false
		for _, x := range attackKindOrder {
			if x == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("分类 %q 有中文名但不在展示顺序里，界面上不会出现", k)
		}
	}
}

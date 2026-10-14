package stun

import (
	"testing"
)

// 下面两个用例要连**真实的公网 STUN 服务器**，结论还取决于所在网络的 NAT 行为——
// 换一个机房、换一个 ISP，结果本来就不一样，失败并不代表代码有问题
// （比如 CI runner 上到 stun.sonetel.net 的 TCP 常常直接被防火墙掐掉）。
//
// 所以它们只在显式全量跑时执行；CI 统一用 `go test -short ./...` 跳过。
// 本地排查 NAT / 打洞问题时再 `go test ./modules/stun/` 全量跑一次。
const rfc5780SkipReason = "依赖公网 STUN 服务器与所在网络的 NAT 行为，-short 下跳过（CI 用 -short）"

func TestRFC5780UDP(t *testing.T) {
	if testing.Short() {
		t.Skip(rfc5780SkipReason)
	}
	server, err := selectRFC5780Server(rfc5780StunServer)
	if err != nil {
		t.Fatalf("selectRFC5780Server 失败: %v", err)
	}
	t.Logf("选中服务器: %s primary=%s other=%s delay=%s",
		server.Name, server.Primary, server.Other, server.Delay)

	t.Run("Mapping", func(t *testing.T) {
		behavior, err := detectUDPMapping(server)
		if err != nil {
			t.Fatalf("detectUDPMapping 失败: %v", err)
		}
		t.Logf("UDP Mapping 行为: %s", behavior)
	})

	t.Run("Filtering", func(t *testing.T) {
		behavior, err := detectUDPFiltering(server)
		if err != nil {
			t.Fatalf("detectUDPFiltering 失败: %v", err)
		}
		t.Logf("UDP Filtering 行为: %s", behavior)
	})
}

func TestRFC5780TCP(t *testing.T) {
	if testing.Short() {
		t.Skip(rfc5780SkipReason)
	}
	server, err := selectRFC5780Server(rfc5780StunServer)
	if err != nil {
		t.Fatalf("selectRFC5780Server 失败: %v", err)
	}
	t.Logf("选中服务器: %s primary=%s other=%s", server.Name, server.Primary, server.Other)

	behavior, err := detectTCPMapping(server)
	if err != nil {
		t.Fatalf("detectTCPMapping 失败: %v", err)
	}
	t.Logf("TCP Mapping 行为: %s", behavior)
}

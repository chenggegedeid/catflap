package stun

import "testing"

func TestNatType(t *testing.T) {
	// 这一条要真的去连公网的 STUN 服务器做 RFC 5780 分类，结果取决于跑测试的
	// 机器所在网络（CGNAT / UDP 被拦 / 对端抽风都会失败），失败不代表代码有问题。
	// 以前 CI 注释里写「-short 会跳过」，但代码里没这回事 —— CI 一直绿只是因为
	// 当时网络通。现在把它落成真的，理由见 modules/stun/test/stun_service_test.go
	// 里的 skipUnlessNetwork。
	if testing.Short() {
		t.Skip("依赖公网 STUN 服务器，-short 下跳过；要真测请去掉 -short")
	}

	udpResult, tcpResult, err := DetectAndClassifyNat()
	if err != nil {
		t.Fatalf("启动时检测 NAT 类型失败: %v", err)
	}
	if udpResult != nil {
		t.Logf("UDP NAT检测结果: type=%s mapping=%s filtering=%s", udpResult.NatType, udpResult.Mapping, udpResult.Filtering)
	}
	if tcpResult != nil {
		t.Logf("TCP NAT检测结果7: type=%s mapping=%s", tcpResult.NatType, tcpResult.Mapping)
	}
}

func TestClassifyTCPNatTypeEIMFallsBackToPortRestricted(t *testing.T) {
	status := &NATStatus{Mapping: MappingEndpointIndependent}

	got := ClassifyTCPNatType(status, "192.168.1.10", "203.0.113.10")
	if got != PortRestricted {
		t.Fatalf("ClassifyTCPNatType() = %q, want %q", got, PortRestricted)
	}
}

func TestClassifyTCPNatTypePublicDirect(t *testing.T) {
	status := &NATStatus{Mapping: MappingEndpointIndependent}

	got := ClassifyTCPNatType(status, "203.0.113.10", "203.0.113.10")
	if got != OpenInternet {
		t.Fatalf("ClassifyTCPNatType() = %q, want %q", got, OpenInternet)
	}
}

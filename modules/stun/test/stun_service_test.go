package test

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/chenggegedeid/catflap/modules/stun"

	"github.com/libp2p/go-reuseport"
	pionstun "github.com/pion/stun"
)

type stunResult struct {
	tcpIP string
	udpIP string
}

// skipUnlessNetwork 网络用例的统一跳过条件。
//
// 这里的用例要真的去连公网的 STUN 服务器，结果取决于**跑测试的这台机器的
// 网络**（CGNAT、UDP 被掐、公司防火墙、对端服务器抽风都会让它失败），
// 失败并不代表代码有问题。
//
// 以前 CI 只在注释里写「-short 会跳过这两个用例」，但代码里根本没有跳过逻辑 ——
// CI 之所以一直绿，只是因为当时网络恰好通。现在把这句话落成真的：
// `go test -short` 跳过，想真测就 `go test ./modules/stun/...`（不带 -short）。
func skipUnlessNetwork(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("依赖公网 STUN 服务器，-short 下跳过；要真测请去掉 -short")
	}
}

func TestReuseportUDPWrite(t *testing.T) {
	skipUnlessNetwork(t)

	srv := "stun.finsterwalder.com:3478"
	localIP := "0.0.0.0"
	localAddr := fmt.Sprintf("%s:0", localIP)

	conn, err := reuseport.Dial("udp", localAddr, srv)
	if err != nil {
		t.Skipf("网络不可达，跳过: %v", err)
	}
	defer conn.Close()

	t.Logf("conn 1类型: %T", conn) // 看看实际类型

	ip, port, err := stun.DoStunHandshake(conn)
	if err != nil {
		t.Fatalf("reuseport UDP STUN 失败: %v", err)
	}
	t.Logf("[reuseport UDP] 公网地址: %s:%d", ip, port)
}

func TestAllSTUNServers(t *testing.T) {
	// 这是一份「哪些公共 STUN 服务器现在还活着」的探测器，不是断言 ——
	// 单台服务器失败只记日志。但仍然要联网，且会并发打十几台外部服务器，
	// 所以一样在 -short 下跳过。
	skipUnlessNetwork(t)

	cfg, err := stun.ReadConfig()
	fmt.Println(cfg)
	if err != nil {
		t.Fatalf("1read 1stun config: %v", err)
	}

	var mu sync.Mutex
	results := make(map[string]*stunResult, len(cfg.StunServerList))
	for _, srv := range cfg.StunServerList {
		results[srv] = &stunResult{}
	}

	for _, proto := range []string{"tcp", "udp"} {
		for _, srv := range cfg.StunServerList {
			proto, srv := proto, srv
			t.Run(proto+"/"+srv, func(t *testing.T) {
				t.Parallel()
				ip, port, delay, err := queryStun(proto, srv)
				if err != nil {
					t.Logf("[FAIL] %s %s err=%v", proto, srv, err)
					return
				}
				t.Logf("[ OK ] %s %s -> %s:%d  %v", proto, srv, ip, port, delay.Round(time.Millisecond))
				mu.Lock()
				if proto == "tcp" {
					results[srv].tcpIP = ip
				} else {
					results[srv].udpIP = ip
				}
				mu.Unlock()
			})
		}
	}

	t.Cleanup(func() {
		type badEntry struct {
			srv    string
			reason string
		}
		var good []string
		var bads []badEntry
		for _, srv := range cfg.StunServerList {
			r := results[srv]
			switch {
			case r.tcpIP == "" && r.udpIP == "":
				bads = append(bads, badEntry{srv, "tcp+udp 均失败"})
			case r.tcpIP == "":
				bads = append(bads, badEntry{srv, "tcp 失败 udp=" + r.udpIP})
			case r.udpIP == "":
				bads = append(bads, badEntry{srv, "udp 失败 tcp=" + r.tcpIP})
			case r.tcpIP != r.udpIP:
				bads = append(bads, badEntry{srv, fmt.Sprintf("ip 不一致 tcp=%s udp=%s", r.tcpIP, r.udpIP)})
			default:
				good = append(good, srv)
			}
		}

		fmt.Println("\n====== TCP/UDP 均通过且 IP 一致 ======")
		out, _ := json.MarshalIndent(map[string][]string{"stunServerList": good}, "", "  ")
		fmt.Println(string(out))

		fmt.Println("\n====== 其他服务器 ======")
		sort.Slice(bads, func(i, j int) bool { return bads[i].srv < bads[j].srv })
		for _, b := range bads {
			fmt.Printf("  %-35s %s\n", b.srv, b.reason)
		}
	})
}

func queryStun(protocol, server string) (string, int, time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout(protocol, server, 3*time.Second)
	if err != nil {
		return "", 0, 0, err
	}
	defer conn.Close()

	msg := pionstun.MustBuild(pionstun.TransactionID, pionstun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return "", 0, 0, err
	}
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", 0, 0, err
	}

	var resp pionstun.Message
	resp.Raw = buf[:n]
	if err := resp.Decode(); err != nil {
		return "", 0, 0, err
	}

	var xorAddr pionstun.XORMappedAddress
	if err := xorAddr.GetFrom(&resp); err != nil {
		return "", 0, 0, err
	}
	return xorAddr.IP.String(), xorAddr.Port, time.Since(start), nil
}

package wol

import (
	"bytes"
	"testing"
)

// TestParseMACAcceptsCommonFormats 三种常见写法都要认。
//
// 用户是从路由器后台 / ipconfig / ifconfig 里复制 MAC 的，分隔符各不相同。
// 不归一化的话，现象是「填了却点不动」，而错误信息还看不出是分隔符的问题。
func TestParseMACAcceptsCommonFormats(t *testing.T) {
	cases := map[string]string{
		"00:11:22:33:44:55":     "00:11:22:33:44:55",
		"00-11-22-33-44-55":     "00:11:22:33:44:55",
		"0011.2233.4455":        "00:11:22:33:44:55",
		"001122334455":          "00:11:22:33:44:55",
		"  00:11:22:AA:bb:Cc  ": "00:11:22:aa:bb:cc",
	}
	for in, want := range cases {
		got, err := ParseMAC(in)
		if err != nil {
			t.Errorf("ParseMAC(%q) 报错: %v", in, err)
			continue
		}
		if got.String() != want {
			t.Errorf("ParseMAC(%q) = %s，期望 %s", in, got.String(), want)
		}
	}
}

func TestParseMACRejectsGarbage(t *testing.T) {
	for _, in := range []string{
		"",
		"00:11:22:33:44",       // 少了两位
		"00:11:22:33:44:55:66", // 多了一位
		"00:11:22:33:44:ZZ",
		"不是MAC",
	} {
		if _, err := ParseMAC(in); err == nil {
			t.Errorf("ParseMAC(%q) 应该报错却通过了", in)
		}
	}
}

// TestBuildMagicPacket 唤醒包的格式：6 个 0xFF + MAC 重复 16 遍。
//
// 这个包格式错了不会有任何报错 —— 目标机器只是不醒，日志里一片安静。
// 所以必须用测试钉住它。
func TestBuildMagicPacket(t *testing.T) {
	mac, err := ParseMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}
	packet, err := BuildMagicPacket(mac)
	if err != nil {
		t.Fatal(err)
	}

	if len(packet) != magicPacketLen {
		t.Fatalf("包长 = %d，期望 %d", len(packet), magicPacketLen)
	}
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			t.Fatalf("第 %d 字节 = %#x，期望 0xFF（同步头）", i, packet[i])
		}
	}
	for i := 0; i < 16; i++ {
		want := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		got := packet[6+i*6 : 6+(i+1)*6]
		if !bytes.Equal(got, want) {
			t.Fatalf("第 %d 段 = %v，期望 %v", i, got, want)
		}
	}
}

func TestBuildMagicPacketRejectsShortMAC(t *testing.T) {
	if _, err := BuildMagicPacket([]byte{0x00, 0x11, 0x22}); err == nil {
		t.Error("MAC 长度不对时应该报错")
	}
}

// TestWakeSendsWithoutError 往本机发一个包，验证整条路径能走通。
//
// 注意这里**只验证「发出去了」**：UDP 发出去成功不代表目标会醒。
// 真正的验证只能靠拿一台真机器来试（见部署文档）。
func TestWakeSendsWithoutError(t *testing.T) {
	if err := Wake("00:11:22:33:44:55", "127.0.0.1", 9); err != nil {
		t.Fatalf("往 127.0.0.1 发唤醒包失败: %v", err)
	}
}

func TestWakeRejectsBadInput(t *testing.T) {
	if err := Wake("", "127.0.0.1", 9); err == nil {
		t.Error("MAC 为空时应该报错")
	}
	if err := Wake("00:11:22:33:44:55", "", 9); err == nil {
		t.Error("广播地址为空时应该报错")
	}
}

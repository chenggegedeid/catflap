package wol

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// magicPacketLen 唤醒包的固定长度：6 字节 0xFF 开头，后接 MAC 重复 16 次。
const magicPacketLen = 6 + 6*16

// ParseMAC 把用户填的 MAC 解析成 6 字节。
//
// 接受三种常见写法（00:11:22:33:44:55 / 00-11-22-33-44-55 / 001122.334455），
// 因为从路由器后台、网卡属性、ifconfig 里复制出来的格式都不一样。
// 不做这个归一化的话，用户会因为「分隔符不对」而点不动，还看不出原因。
func ParseMAC(s string) (net.HardwareAddr, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, errors.New("MAC 地址不能为空")
	}

	// 只留十六进制字符，分隔符一律忽略
	var hex []byte
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			hex = append(hex, c)
		case c == ':' || c == '-' || c == '.' || c == ' ':
			continue
		default:
			return nil, fmt.Errorf("MAC 地址里有非法字符 %q", string(c))
		}
	}
	if len(hex) != 12 {
		return nil, fmt.Errorf("MAC 地址应该是 12 位十六进制，实际解析出 %d 位", len(hex))
	}

	mac := make(net.HardwareAddr, 6)
	for i := range mac {
		v, err := parseHexByte(hex[i*2], hex[i*2+1])
		if err != nil {
			return nil, err
		}
		mac[i] = v
	}
	return mac, nil
}

func parseHexByte(hi, lo byte) (byte, error) {
	h, ok := hexVal(hi)
	if !ok {
		return 0, fmt.Errorf("MAC 地址里有非法字符 %q", string(hi))
	}
	l, ok := hexVal(lo)
	if !ok {
		return 0, fmt.Errorf("MAC 地址里有非法字符 %q", string(lo))
	}
	return h<<4 | l, nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// BuildMagicPacket 构造唤醒包：6 个 0xFF + MAC 重复 16 遍。
//
// 单独拆成一个函数是为了能写单测 —— 唤醒包是全项目里少数几个
// 「格式错了就必然不生效、而且没有任何报错」的东西。
func BuildMagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("MAC 地址必须是 6 字节，实际 %d 字节", len(mac))
	}
	packet := make([]byte, 0, magicPacketLen)
	for i := 0; i < 6; i++ {
		packet = append(packet, 0xFF)
	}
	for i := 0; i < 16; i++ {
		packet = append(packet, mac...)
	}
	return packet, nil
}

// Wake 向目标发一个唤醒包。
//
// 走的是 UDP：WOL 只需要把那个包送到二层广播地址上，不需要连接、
// 也没有响应可等。发成功**不代表目标醒过来**（目标可能没开 WOL、
// 广播地址填错、跨了网段），所以别把它当成「成功」告诉用户。
func Wake(mac string, broadcast string, port int) error {
	hw, err := ParseMAC(mac)
	if err != nil {
		return err
	}
	packet, err := BuildMagicPacket(hw)
	if err != nil {
		return err
	}
	if broadcast == "" {
		return errors.New("广播地址不能为空")
	}
	if port <= 0 {
		port = 9
	}

	addr := net.JoinHostPort(broadcast, fmt.Sprint(port))
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("广播地址 %q 解析失败: %w", addr, err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("发送唤醒包失败: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Write(packet); err != nil {
		return fmt.Errorf("发送唤醒包失败: %w", err)
	}
	return nil
}

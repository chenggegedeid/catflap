/*
Package device 设备表 —— 内网里那些机器。

**它是穿透、反向代理、端口转发共用的唯一一份**：
同一台机器（同一个 IP）只在这里存一次，三个功能都引用它。

之前设备嵌在穿透的配置里（`stunConfig.json` 的 devices），反代要用就得
依赖 stun —— 而 stun 启动时要跑几秒网络探测，反代的监听不该等它。
所以把设备下沉成公共模块，依赖方向变成：

	stun  ─┐
	       ├→ device
	proxy ─┘

两边互不依赖，都只依赖这个模块。

**这个模块只管机器，不管机器上跑了什么。** 服务（穿透服务、反代站点、
端口转发）由各自的模块存，通过 IP 或 deviceId 关联过来。
把服务也塞进来会让公共模块认识业务，那是以前踩过的坑（v1.7 抽共用扫描
弹窗时定的规矩：共用组件不判断业务）。
*/
package device

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Device 一台内网机器。
//
// IP 是它的标识 —— 归属关系（这个服务在哪台机器上）由 IP 推导，
// 不需要在服务上再存一个 deviceId。
type Device struct {
	DeviceID uint   `json:"id"`   // 沿用穿透那边的字段名，减少迁移时的改动
	Name     string `json:"name"` // "本机" / "群晖NAS" / "树莓派"
	IP       string `json:"ip"`   // 设备 IP

	// SortOrder 界面上的排列顺序，小的在前。
	//
	// 只影响显示，不影响任何转发行为。0 表示没排过 ——
	// 排序时按 (SortOrder, ID) 排，没排过的就按 ID 自然顺序，
	// 老配置升级后顺序不变。
	SortOrder int       `json:"sortOrder,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Runtime 设备表的运行时
type Runtime struct {
	mu      sync.RWMutex
	Devices []Device
}

var runtime = &Runtime{}

func init() {
	// 读配置 + 迁移（老配置从 stunConfig.json 搬过来）都在 ReadConfig 里
	if err := ReadConfig(); err != nil {
		// 设备表读不出来不该让整个程序起不来 —— 退化为"没有设备"，
		// 用户可以在界面上加。但要在日志里说清楚。
		logInitError(err)
	}
}

// ===== 查询 =====

// List 全部设备，按 ID 排序
func List() []Device {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	out := make([]Device, len(runtime.Devices))
	copy(out, runtime.Devices)
	sortDevices(out)
	return out
}

// sortDevices 按 (SortOrder, ID) 排。
// 没设过 SortOrder 的都是 0，同级再按 ID —— 老配置升级后顺序不变。
func sortDevices(list []Device) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].SortOrder != list[j].SortOrder {
			return list[i].SortOrder < list[j].SortOrder
		}
		return list[i].DeviceID < list[j].DeviceID
	})
}

// Get 按 ID 取
func Get(id uint) (Device, bool) {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	for _, d := range runtime.Devices {
		if d.DeviceID == id {
			return d, true
		}
	}
	return Device{}, false
}

// FindByIP 按 IP 取。这是最常用的一条 ——
// 服务（站点、端口转发）通过后端地址的 IP 归属到设备。
//
// 比对前做归一化（去空格、转小写），IPv6 的写法差异也能容错。
func FindByIP(ip string) (Device, bool) {
	want := normalizeIP(ip)
	if want == "" {
		return Device{}, false
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	for _, d := range runtime.Devices {
		if normalizeIP(d.IP) == want {
			return d, true
		}
	}
	return Device{}, false
}

// ExistsIP 这个 IP 是不是已经有设备了（加设备时查重）
func ExistsIP(ip string) bool {
	_, ok := FindByIP(ip)
	return ok
}

// ===== 修改 =====

// Add 加一台设备，返回带 ID 的结果。IP 重复会拒绝 ——
// 同一个 IP 存两条，按 IP 归属时就不知道该算谁的。
func Add(name, ip string) (Device, error) {
	name = strings.TrimSpace(name)
	ip = normalizeIP(ip)
	if ip == "" {
		return Device{}, ErrEmptyIP
	}
	if name == "" {
		name = ip // 没起名字就用 IP，总比空着强
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if hasIPLocked(ip) {
		return Device{}, ErrIPExists
	}

	d := Device{
		DeviceID:  nextIDLocked(),
		Name:      name,
		IP:        ip,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	runtime.Devices = append(runtime.Devices, d)

	// 已持写锁，必须用不加锁的那个版本（RWMutex 不可重入）
	if err := writeConfigLocked(runtime.Devices); err != nil {
		// 落盘失败要把内存里的也撤掉，否则界面上看到一条重启就没的记录
		runtime.Devices = runtime.Devices[:len(runtime.Devices)-1]
		return Device{}, err
	}
	return d, nil
}

// Update 改名字或 IP
func Update(id uint, name, ip string) (Device, error) {
	name = strings.TrimSpace(name)
	ip = normalizeIP(ip)
	if ip == "" {
		return Device{}, ErrEmptyIP
	}
	if name == "" {
		name = ip
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	for i := range runtime.Devices {
		if runtime.Devices[i].DeviceID != id {
			continue
		}
		// IP 换了要查重（排除自己）：新 IP 已被另一台占着时不让改。
		// 这里返回的 IP 错误格式是固定的，api 层包装成 409 + 提示文案，
		// 让用户去「设备」页删掉旧的那台再来。
		if normalizeIP(runtime.Devices[i].IP) != ip && hasIPLocked(ip) {
			return Device{}, fmt.Errorf("%s %s: %w", ErrIPExists, ip, ErrIPExists)
		}
		old := runtime.Devices[i]
		runtime.Devices[i].Name = name
		runtime.Devices[i].IP = ip
		runtime.Devices[i].UpdatedAt = time.Now()

		if err := writeConfigLocked(runtime.Devices); err != nil {
			runtime.Devices[i] = old
			return Device{}, err
		}
		// 落盘成功后才同步三个模块里缓存的 IP —— 设备表里的 IP 是单一真相。
		// Site.Backend / Forward.Target 形如 ip:port，里面那段 IP 也要跟着换，
		// 否则下次 hydrate 又用新设备的 IP 去匹配 —— 反而匹配不上、产生"未归设备"。
		if old.IP != ip {
			if err := syncIpsLocked(old.IP, ip); err != nil {
				// 同步失败不致命：设备表里已经是新 IP 了（人不会再进这台设备的旧地址），
				// 只是站点/转发的旧 IP 还在 → 暂时未归设备 → 用户得去那条规则改 Target/Backend。
				// 把这个隐患告诉 api 层（以 ip 冲突错误的形式带上原始失败）。
				return runtime.Devices[i], fmt.Errorf("IP 已改为 %s，但同步旧规则的地址失败：%w", ip, err)
			}
		}
		return runtime.Devices[i], nil
	}
	return Device{}, ErrNotFound
}

// Delete 删一台设备。
//
// 调用方（各业务模块）负责先清理挂在这台设备上的服务 ——
// 这个模块不认识服务，删不干净也只能由它们来保证。
func Delete(id uint) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	for i := range runtime.Devices {
		if runtime.Devices[i].DeviceID != id {
			continue
		}
		old := runtime.Devices[i]
		runtime.Devices = append(runtime.Devices[:i], runtime.Devices[i+1:]...)

		if err := writeConfigLocked(runtime.Devices); err != nil {
			runtime.Devices = append(runtime.Devices[:i], old)
			return err
		}
		return nil
	}
	return ErrNotFound
}

// ===== 内部 =====

func nextIDLocked() uint {
	var max uint
	for _, d := range runtime.Devices {
		if d.DeviceID > max {
			max = d.DeviceID
		}
	}
	return max + 1
}

func hasIPLocked(ip string) bool {
	for _, d := range runtime.Devices {
		if normalizeIP(d.IP) == ip {
			return true
		}
	}
	return false
}

// normalizeIP 归一化：去空格、去 IPv6 方括号、转小写
func normalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")
	return strings.ToLower(ip)
}

// Reorder 按给定的 ID 顺序重排，并落盘。
//
// 传进来的顺序就是界面上的顺序（第一个排最前）。
// 只认已有设备的 ID，多余的忽略 —— 前端给的顺序可能因为
// 并发（别人刚删了一台）而不全，不该因此报错。
func Reorder(ids []uint) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	byID := map[uint]*Device{}
	for i := range runtime.Devices {
		byID[runtime.Devices[i].DeviceID] = &runtime.Devices[i]
	}

	next := 1
	for _, id := range ids {
		d, ok := byID[id]
		if !ok {
			continue
		}
		d.SortOrder = next
		next++
		delete(byID, id) // 同一 ID 出现两次时只认第一次
	}
	// 没在列表里的（前端可能漏给）排在后面，保持相对顺序
	rest := make([]*Device, 0, len(byID))
	for _, d := range byID {
		rest = append(rest, d)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].DeviceID < rest[j].DeviceID })
	for _, d := range rest {
		d.SortOrder = next
		next++
	}

	return writeConfigLocked(runtime.Devices)
}

// =====================================================================
// 跨模块联动（仅 IP 变化时）
// =====================================================================

// syncIpsLocked 改设备 IP 时调用：把三个模块里仍然按旧 IP 写的规则
// （Site.Backend / Forward.Target）里的那段 IP 替换成新 IP。
//
// 必须在 device 写盘**成功后**调：设备表里的 IP 是单一真相，三个模块的
// 配置只是缓存 / 派生视图。顺序反了的话一边改一边错。
//
// 调用者持 device 写锁；这里要拿 proxy/stun 的写锁 ——
// 锁顺序要先 stun/proxy 再 device 才不会死锁（proxy/stun 拿设备表也是先
// 设备后业务），所以这里反向拿到锁。
//
// 当前实现：仅替换 Site.Backend / Forward.Target 内的 IP 段，Service 不用
// 改（它用 DeviceID 关联）。stun 的 services 段里没有 IP 字段。
func syncIpsLocked(oldIP, newIP string) error {
	if oldIP == "" || oldIP == newIP {
		return nil
	}
	// 设备锁是调用者拿的；proxy / stun 锁在这里拿 —— 不会循环依赖，
	// 因为 device 不反向调这两个模块。
	if onIPChanged != nil {
		if err := onIPChanged(oldIP, newIP); err != nil {
			return err
		}
	}
	return nil
}

// onIPChanged 由依赖方注入：设备 IP 变化时同步它们缓存的 IP 字符串。
//
// **为什么用回调而不是直接 import**：依赖方向是 stun/proxy → device，device 不能反过来依赖它们
// （会成环，编译直接报 import cycle）。和 core/lanscan 一样，用注入把"业务知识"留在业务侧。
var onIPChanged func(oldIP, newIP string) error

// SetIPChangedHook 由上层（routers 或 app 初始化时）注册 IP 同步逻辑。
// 传 nil 表示不同步（单测 / 只需要设备表的场景）。
func SetIPChangedHook(fn func(oldIP, newIP string) error) {
	onIPChanged = fn
}

// EnsureLocalhost 启动时调：把本机 IP（最有可能的 LAN 网卡那个）加进设备表。
//
// 为什么不依赖扫描：扫描是用户主动行为，本机自己部署的 catflap 永远应该
// 在列表里（哪怕用户没扫过）—— 否则会出现「我装了 catflap，但设备页找不到」。
//
// 已存在则不动；扫描到的设备有别的名字，更新名字反而把用户改过的覆盖了。
func EnsureLocalhost() {
	ips := localIPs()
	if len(ips) == 0 {
		return
	}
	// 只加"最像 LAN"的那一个：docker 网桥（172.17/172.18/172.21）也会被
	// localIPs 枚举到，全加进去会在设备页刷出一堆本机条目。
	ip := ips[0]
	if GetByIP(ip) != nil {
		return
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "本机"
	}
	// 失败不致命（写盘问题等下次重启会再试）
	_, _ = Add(host, ip)
}

// GetByIP 按 IP 查一台设备（IP 已规范化）。
// 老数据按 IP 字符串关联设备的代码路径依赖这个 —— 找不到时调用方
// 应继续用 IP 字符串匹配，而不是把这条规则标成"未归设备"。
func GetByIP(ip string) *Device {
	ip = normalizeIP(ip)
	if ip == "" {
		return nil
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	for i := range runtime.Devices {
		if runtime.Devices[i].IP == ip {
			d := runtime.Devices[i]
			return &d
		}
	}
	return nil
}

// localIPs 枚举本机所有非回环 IPv4，按"最像 LAN"排序：192.168 > 10 > 172.16 > 其他。
// EnsureLocalhost 用它来挑本机的首选 IP 自动加进设备表。
// 只取一个最像 LAN 的就够了：同一台机器被加一次，避免「172.18 也加一台」刷屏。
func localIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var v4 []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			v4 = append(v4, ip.To4().String())
		}
	}
	// 选最像 LAN 的
	sort.SliceStable(v4, func(i, j int) bool {
		return lanScore(v4[i]) > lanScore(v4[j])
	})
	return v4
}

func lanScore(ip string) int {
	if strings.HasPrefix(ip, "192.168.") {
		return 4
	}
	if strings.HasPrefix(ip, "10.") {
		return 3
	}
	if strings.HasPrefix(ip, "172.") {
		return 2
	}
	return 1
}

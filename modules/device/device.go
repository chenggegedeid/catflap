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
		// IP 换了要查重（排除自己）
		if normalizeIP(runtime.Devices[i].IP) != ip && hasIPLocked(ip) {
			return Device{}, ErrIPExists
		}
		old := runtime.Devices[i]
		runtime.Devices[i].Name = name
		runtime.Devices[i].IP = ip
		runtime.Devices[i].UpdatedAt = time.Now()

		if err := writeConfigLocked(runtime.Devices); err != nil {
			runtime.Devices[i] = old
			return Device{}, err
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

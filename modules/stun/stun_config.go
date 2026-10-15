package stun

import (
	device "github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/modules/stun/model"
	"github.com/chenggegedeid/catflap/utils/utilsFile"
	"os"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/stunConfig.json"

// 读取stun_config 配置文件
func ReadConfig() (model.Config, error) {
	var config model.Config

	//检测文件是否存在
	if fileInfo, err := os.Stat(ConfigPath); os.IsNotExist(err) || fileInfo.Size() == 0 {
		//不存在创建空配置文件
		return createConfig()

	} else {
		//文件存在读取配置文件
		config, err = utilsFile.ReadJsonFile[model.Config](ConfigPath)
		if err != nil {
			logrus.Error("Config读取失败：", err)
			return config, err
		}
		if migrateServices(&config) {
			// 拆完了要立刻写一次盘：不然 stunConfig.json 里还留着老的设备数据，
			// 虽然下次启动会重新拆（幂等，不会出错），但文件里一直有一份
			// 已经不用的 devices，看着像两份数据打架。
			// 这里写盘会顺带把 devices 字段清掉（dehydrate 的效果）。
			if err := UpdateConfig(config); err != nil {
				logrus.Errorf("[stun] 迁移后落盘失败（不影响运行，下次启动会重试）：%v", err)
			}
		}
		hydrateDevices(&config)
		normalizeServices(&config)

	}
	return config, nil

}

// normalizeServices 清掉配置文件里那些自相矛盾、必然连不上的组合。
//
// 目前只有一条：没勾「洞口终结 TLS」却勾了「转发给内网时也用 HTTPS」。
// 洞口不终结时这个洞是纯字节管道，浏览器的 TLS 直达内网服务；
// 再让 Catflap 去 tls.Dial，等于把浏览器的 ClientHello 当明文塞进
// Catflap 自己那条 TLS，双层 TLS 必坏，浏览器报 ERR_SSL_PROTOCOL_ERROR。
// 新版 UI 已经不让这么勾，但旧配置文件里可能存着，在这里一次性收敛，
// 免得 PublicScheme、首页地址、实际转发三处各说各话。
func normalizeServices(config *model.Config) {
	for i := range config.Devices {
		for j := range config.Devices[i].Services {
			svc := &config.Devices[i].Services[j]
			if svc.BackendHTTPS && !svc.TLSTerminate {
				svc.BackendHTTPS = false
				logrus.Warnf("服务 %s 勾了内网 HTTPS 但洞口没终结 TLS，这个组合必然握手失败，已按纯管道处理", svc.Name)
			}
		}
	}
}

// DefaultSTUNServers 内置的 STUN 服务器列表。
//
// 两处会用它：首次创建配置、以及用户在界面上点「恢复内置列表」。
// 抽成变量是为了不让这两处各写一份、然后慢慢不一致。
//
// ⚠️ 这份列表的来历是「上游默认带的那批」，**不是我们实测过的**。
// 实测要靠界面上那次测速：保存/恢复后程序会立即重测并选出最优，
// 用户再把自己网络里连不上的剔掉。别把它当成「保证可用」的清单。
var DefaultSTUNServers = []string{
	"stun.annatel.net:3478",
	"stun.antisip.com:3478",
	"stun.commpeak.com:3478",
	"stun.dcalling.de:3478",
	"stun.freeswitch.org:3478",
	"stun.ipfire.org:3478",
	"stun.sip.us:3478",
	"stun.siplogin.de:3478",
	"stun.sonetel.net:3478",
	"stun.voip.blackberry.com:3478",
	"stun.nextcloud.com:443",
	"stun.flashdance.cx:3478",
	"fwa.lifesizecloud.com:3478",
	"stun.nextcloud.com:3478",
	"stun.radiojar.com:3478",
	"stun.sonetel.com:3478",
	"stun.voipgate.com:3478",
}

// DefaultSTUNServersCopy 返回内置列表的一份副本。
//
// 直接把 DefaultSTUNServers 赋给配置的话，用户后续编辑会**改到这个包级变量**，
// 于是「恢复内置列表」恢复出来的会是上次被改过的那份 —— 这类 bug 只在
// 「改过一次再恢复」时才出现，很难从现象倒推。
func DefaultSTUNServersCopy() []string {
	out := make([]string, len(DefaultSTUNServers))
	copy(out, DefaultSTUNServers)
	return out
}

func createConfig() (model.Config, error) {
	var config model.Config
	// 首次创建，设置创建时间
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	// 初始化stun服务器
	config.StunServerList = DefaultSTUNServersCopy()

	// 确保 config 目录存在
	if err := utilsFile.EnsurePrivateDir("config"); err != nil {
		logrus.Error("创建config目录失败：", err)
		return config, err
	}

	// 写入一个空的配置文件
	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("Config写入失败：", err)
		return config, err
	}
	return config, nil
}

// UpdateConfig 更新stun配置文件
func UpdateConfig(config model.Config) error {
	const ConfigPath = "config/stunConfig.json"

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 写盘前把内存里的设备树拆回独立的服务列表（见 dehydrateDevices）
	dehydrateDevices(&config)

	// 写入配置文件
	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("Config写入失败：", err)
		return err
	}

	logrus.Info("STUN配置文件已更新")
	return nil
}

// migrateServices 把老配置里嵌在设备下的服务搬到 Config.Services。
//
// 为什么要搬：设备已经归 modules/device 管了（穿透/反代/端口转发共用
// 那一份），服务嵌在设备里就跟着一起搬走了 —— 得有个自己的地方放。
//
// **只在读出来的老配置里 Devices[].Services 非空时执行**（幂等）：
// 搬完把 Devices 清空，之后不再写回。
func migrateServices(config *model.Config) bool {
	if config == nil || len(config.Devices) == 0 {
		return false
	}

	// 已经有拆好的服务了，说明迁移过了
	if len(config.Services) > 0 {
		config.Devices = nil
		return false
	}

	out := make([]model.Service, 0, 8)
	for _, d := range config.Devices {
		for _, svc := range d.Services {
			svc.DeviceID = d.DeviceID
			out = append(out, svc)
		}
	}

	config.Services = out
	// 设备不再存在这里（主人在 devices.json），清掉避免以后又迁一次
	config.Devices = nil

	if len(out) > 0 {
		logrus.Infof("[stun] 已把 %d 个服务从设备里拆到独立列表（设备归 modules/device 管）", len(out))
	}
	return true
}

// ===== 设备树与服务列表的互相转换 =====
//
// 设备的"主人"是 modules/device（穿透/反代/端口转发共用一份），
// 服务存在自己的 Config.Services 里。
//
// 但穿透的现有代码（调度器、查找、通知……几十处）都是按
// `设备 → 它的服务列表` 这棵树来写的。为它们全部改一遍风险太大，
// 所以这里做一层转换：
//
//	读配置 → hydrateDevices：device 表 + Services  → 组装成 Devices 树（内存）
//	写配置 → dehydrateDevices：Devices 树 → 拆回 Services（存盘）
//
// **Config.Devices 因此只是内存视图，不落盘**（json tag 带 omitempty 且
// 写盘前会被清空）。这样一来那几十处引用一行都不用动，
// 而设备数据在文件里仍然只有一份。

// hydrateDevices 用设备表和服务列表组装出设备树
func hydrateDevices(config *model.Config) {
	if config == nil {
		return
	}
	// 设备基础信息从公共模块拿（唯一的一份）
	devs := deviceList()
	byID := map[uint][]model.Service{}
	for _, svc := range config.Services {
		byID[svc.DeviceID] = append(byID[svc.DeviceID], svc)
	}

	out := make([]model.Device, 0, len(devs))
	for _, d := range devs {
		svcs := byID[d.DeviceID]
		// 按界面顺序排（SortOrder 小的在前），没排过的按 ID ——
		// 拖动排序存在服务上，hydrate 时得按它排，否则拖完刷新又回去了
		sort.SliceStable(svcs, func(i, j int) bool {
			if svcs[i].SortOrder != svcs[j].SortOrder {
				return svcs[i].SortOrder < svcs[j].SortOrder
			}
			return svcs[i].ID < svcs[j].ID
		})
		out = append(out, model.Device{
			DeviceID:  d.DeviceID,
			Name:      d.Name,
			IP:        d.IP,
			Services:  svcs,
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		})
	}
	config.Devices = out
}

// dehydrateDevices 把设备树拆回独立的服务列表，并清空设备字段（不落盘）
func dehydrateDevices(config *model.Config) {
	if config == nil {
		return
	}

	// **服务已经独立存储时，不要从设备树重建** —— 会把服务清空。
	//
	// 这个分支是给"刚做完迁移"用的：migrateServices 把服务拆到 Services
	// 之后会清空 Devices，紧接着 UpdateConfig 落盘又会走到这里。
	// 如果无条件从设备树重建，刚拆出来的服务就会被覆盖成空 ——
	// 端到端实测踩到过一次，3 个服务全丢。
	if len(config.Devices) == 0 && len(config.Services) > 0 {
		config.Devices = nil
		return
	}

	out := make([]model.Service, 0, 8)
	for _, d := range config.Devices {
		for _, svc := range d.Services {
			svc.DeviceID = d.DeviceID
			out = append(out, svc)
		}
	}
	config.Services = out
	config.Devices = nil
}

// deviceModel 公共设备模块的 Device 类型别名，
// 让下面的转换代码读起来不纠结"这个 Device 是哪个包的"
type deviceModel = device.Device

// deviceList 取设备列表。
// 单列一个函数是因为它可能在 device 模块还没初始化完时被调用（启动竞态），
// 那时返回空列表而不是崩。
func deviceList() []deviceModel {
	defer func() {
		// 兜底：任何意外都不该让读配置失败
		_ = recover()
	}()
	return device.List()
}

// ReorderServices 拖动排序后保存**某台设备下**的服务顺序。
//
// 只动这一台设备的服务，别的设备不受影响 —— 界面上就是"这台机器上的
// 服务换个顺序"。
//
// **必须改 Runtime.Config.Devices（设备树），不能只改 Services 列表** ——
// 这两份是同一份数据的两种形态（树是内存视图，列表是落盘形态），
// 服务的新增/修改走的是树那条路，排序也走树才一致。
// 只改列表的话，下一步 hydrate 会拿列表重建树，把树里（真正新加的）
// 服务冲掉 —— 实测踩到过：拖一下排序，服务少了一个。
func ReorderServices(deviceID uint, svcIDs []uint) error {
	for i := range Runtime.Config.Devices {
		if Runtime.Config.Devices[i].DeviceID != deviceID {
			continue
		}
		svcs := Runtime.Config.Devices[i].Services
		byID := map[uint]int{}
		for j := range svcs {
			byID[svcs[j].ID] = j
		}
		next := 1
		for _, id := range svcIDs {
			j, ok := byID[id]
			if !ok {
				continue
			}
			svcs[j].SortOrder = next
			next++
			delete(byID, id)
		}
		// 没在列表里的排后面，保持相对顺序
		for j := range svcs {
			if _, ok := byID[svcs[j].ID]; !ok {
				continue
			}
			svcs[j].SortOrder = next
			next++
		}
		// 改完 SortOrder 还要**真的把 slice 排一遍** ——
		// 界面读的是这个 slice 的顺序，光改字段不动顺序等于没排
		sort.SliceStable(svcs, func(a, b int) bool {
			if svcs[a].SortOrder != svcs[b].SortOrder {
				return svcs[a].SortOrder < svcs[b].SortOrder
			}
			return svcs[a].ID < svcs[b].ID
		})
		break
	}

	return UpdateConfig(Runtime.Config)
}

// RefreshDevices 重新按公共设备表组装设备树。
//
// **什么时候需要**：设备的"主人"是 modules/device，在设备页增删改之后，
// stun 的 Runtime.Config.Devices（内存视图）还是旧的 —— 表现是
// "刚加的设备，穿透页看不到，加服务说设备不存在"，要重启才好。
//
// 所以设备页改完设备后，api 层要调一次这个。
func RefreshDevices() {
	hydrateDevices(&Runtime.Config)
}

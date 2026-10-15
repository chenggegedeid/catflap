package device

import (
	"errors"
	"os"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

// ConfigPath 设备表。从 stunConfig.json 独立出来，三个功能共用这一份。
const ConfigPath = "config/devices.json"

var (
	ErrNotFound = errors.New("设备不存在")
	ErrIPExists = errors.New("这个 IP 已经有设备了")
	ErrEmptyIP  = errors.New("IP 不能为空")
)

// ReadConfig 读设备表；文件不存在就尝试从穿透的老配置迁移过来。
func ReadConfig() error {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		// 新装的实例：没有老配置可搬，直接建一份空的
		return migrateFromStun()
	}

	cfg, err := utilsFile.ReadJsonFile[[]Device](ConfigPath)
	if err != nil {
		return err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	// 配置文件可能被人手改成 null，兜一下
	if cfg == nil {
		cfg = []Device{}
	}
	runtime.Devices = cfg
	return nil
}

// WriteConfig 落盘（公开版本，自己加锁取快照）
func WriteConfig() error {
	runtime.mu.RLock()
	snapshot := make([]Device, len(runtime.Devices))
	copy(snapshot, runtime.Devices)
	runtime.mu.RUnlock()

	return writeConfigLocked(snapshot)
}

// writeConfigLocked 落盘（**调用方已持锁**时用这个）。
//
// 为什么分成两个版本：sync.RWMutex **不可重入** ——
// 拿着写锁再调公开版的 WriteConfig（它要拿读锁）会直接死锁。
// 踩过一次，测试整包卡住不返回。
func writeConfigLocked(snapshot []Device) error {
	if err := utilsFile.EnsurePrivateDir("config"); err != nil {
		return err
	}
	return utilsFile.WriteJsonFile(ConfigPath, snapshot)
}

// migrateFromStun 把老配置里嵌在 stunConfig.json 的设备搬过来。
//
// **只在 devices.json 不存在时执行一次**（幂等）：
// 搬完 stunConfig 里的 devices 字段留空，之后不再写回。
//
// 两个要点：
//   - **设备 ID 必须原样保留** —— 穿透服务是靠 deviceId 定位的，
//     ID 变了服务就认不出自己的机器了
//   - 服务（Device.Services）**不搬** —— 那是穿透的业务数据，
//     由 stun 自己拆到它配置里的 services 列表（见 stun 的迁移）
func migrateFromStun() error {
	legacy, err := readLegacyStunDevices()
	if err != nil || len(legacy) == 0 {
		// 没有老配置，或者读不出来 —— 按"没有设备"继续运行。
		//
		// **刻意不在这里创建空文件**：init() 会在任何引用本包的进程启动时
		// 跑（包括 go test），无条件写盘会在源码目录里留下 configure 垃圾。
		// 等用户真的加第一台设备时再写，见 Add。
		runtime.mu.Lock()
		runtime.Devices = []Device{}
		runtime.mu.Unlock()
		return nil
	}

	runtime.mu.Lock()
	runtime.Devices = legacy
	runtime.mu.Unlock()

	if err := WriteConfig(); err != nil {
		return err
	}
	logrus.Infof("[device] 已从穿透的老配置迁移 %d 台设备到 %s（ID 保持不变）",
		len(legacy), ConfigPath)
	return nil
}

// legacyStunConfig 老 stunConfig.json 里我们关心的那部分。
//
// 只声明需要的字段，不去 import stun 的类型 ——
// 那样就成了 device 依赖 stun，方向反了。
type legacyStunConfig struct {
	Devices []struct {
		DeviceID  uint      `json:"id"`
		Name      string    `json:"name"`
		IP        string    `json:"ip"`
		CreatedAt time.Time `json:"createdAt"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"devices"`
}

// stunConfigPath 穿透的配置文件（只在这里出现一次，避免散落字符串）
const stunConfigPath = "config/stunConfig.json"

func readLegacyStunDevices() ([]Device, error) {
	fi, err := os.Stat(stunConfigPath)
	if err != nil || fi.Size() == 0 {
		return nil, err
	}
	// 泛型版本就够用：老结构是本地声明的，不需要额外的解码入口
	old, err := utilsFile.ReadJsonFile[legacyStunConfig](stunConfigPath)
	if err != nil {
		return nil, err
	}
	if len(old.Devices) == 0 {
		return nil, nil
	}

	out := make([]Device, 0, len(old.Devices))
	for _, d := range old.Devices {
		out = append(out, Device{
			DeviceID:  d.DeviceID,
			Name:      d.Name,
			IP:        d.IP,
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		})
	}
	return out, nil
}

// logInitError 设备表读不出来时记日志。
// 单独一个函数是为了让 device 包不去碰 logrus 以外更复杂的依赖。
func logInitError(err error) {
	logrus.Errorf("[device] 设备表初始化失败，将按「没有设备」继续运行：%v", err)
}

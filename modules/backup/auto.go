package backup

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
)

/**
 * 定时备份：按配置每隔 N 小时把配置打成一份 zip 存到本机，只留最近几份。
 *
 * # 为什么存本机而不是自动上传
 *
 * 自动上传要么接对象存储（要用户配密钥，备份功能本身反而变复杂），
 * 要么发到某个我们不该假设的地方。存本机是最不需要假设的方案：
 * 文件在 `data/backup/` 下，用户想同步到哪儿自己用 syncthing / rclone 都行。
 *
 * # 为什么用 ticker 而不是 cron
 *
 * 只需要"每隔 N 小时"，不需要"每天凌晨 3 点"。ticker 更简单，
 * 也避免了时区/夏令时那堆问题。
 */

// BackupDir 定时备份的存放目录
const BackupDir = "data/backup"

var stopScheduler context.CancelFunc

/**
 * StartSchedule 启动定时备份循环。
 *
 * 重复调用是安全的：会先停掉上一个。配置变了（间隔/开关）也走这里重启。
 */
func StartSchedule() {
	StopSchedule()

	cfg, err := LoadSchedule()
	if err != nil {
		logrus.Warnf("[backup] 读定时备份配置失败，不启动：%v", err)
		return
	}
	if !cfg.Enabled {
		return
	}

	interval := time.Duration(cfg.IntervalHours) * time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	stopScheduler = cancel

	go func() {
		logrus.Infof("[backup] 定时备份已启动，每 %d 小时一次（保留最近 %d 份）", cfg.IntervalHours, cfg.KeepCount)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logrus.Info("[backup] 定时备份已停止")
				return
			case <-ticker.C:
				if err := runAutoBackup(); err != nil {
					logrus.Warnf("[backup] 定时备份失败：%v", err)
				}
			}
		}
	}()
}

// StopSchedule 停掉定时备份（没启动过就什么也不做）
func StopSchedule() {
	if stopScheduler != nil {
		stopScheduler()
		stopScheduler = nil
	}
}

/**
 * runAutoBackup 执行一次定时备份：打包 → 落盘 → 清理旧的 → 回写状态。
 */
func runAutoBackup() error {
	cfg, err := LoadSchedule()
	if err != nil {
		return err
	}

	out, err := Export(ExportOptions{Modules: cfg.Modules})
	if err != nil {
		_ = saveLastResult(cfg, "", err.Error())
		return err
	}

	if err := os.MkdirAll(BackupDir, 0o755); err != nil {
		_ = saveLastResult(cfg, "", err.Error())
		return err
	}

	dest := filepath.Join(BackupDir, out.Filename)
	if err := os.WriteFile(dest, out.Data, 0o600); err != nil {
		_ = saveLastResult(cfg, "", err.Error())
		return err
	}

	// 清理超出保留数量的旧备份
	if err := pruneBackups(cfg.KeepCount); err != nil {
		// 清理失败不影响这次备份成功，记日志即可
		logrus.Warnf("[backup] 清理旧备份失败：%v", err)
	}

	logrus.Infof("[backup] 定时备份完成：%s", dest)
	return saveLastResult(cfg, dest, "成功")
}

/**
 * pruneBackups 只留最近 keep 份。
 *
 * 按文件名排序而不是修改时间：我们自己的文件名带时间戳
 * （catflap-backup-20060102-150405.zip），字典序就是时间序，
 * 不受文件被拷贝/恢复导致 mtime 变化的影响。
 */
func pruneBackups(keep int) error {
	entries, err := os.ReadDir(BackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".zip" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) <= keep {
		return nil
	}

	sort.Strings(names) // 老的在前面
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(BackupDir, name)); err != nil {
			log.Printf("[backup] 删除旧备份 %s 失败：%v", name, err)
		}
	}
	return nil
}

// saveLastResult 把这次执行的时间与结果写回配置（只作展示用）
func saveLastResult(cfg ScheduleConfig, file, result string) error {
	cfg.LastRunAt = time.Now()
	cfg.LastResult = result
	if file != "" {
		cfg.LastResult = "成功：" + filepath.Base(file)
	}
	return SaveSchedule(cfg)
}

// RunAutoBackupForAPI 给「立即备份」按钮用：导出到 API 层的入口
func RunAutoBackupForAPI() error { return runAutoBackup() }

// AutoBackupFile 一份已生成的备份文件
type AutoBackupFile struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
}

// ListAutoBackups 列出 data/backup 下的备份，新的在前
func ListAutoBackups() ([]AutoBackupFile, error) {
	entries, err := os.ReadDir(BackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []AutoBackupFile{}, nil
		}
		return nil, err
	}
	out := make([]AutoBackupFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".zip" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, AutoBackupFile{Name: e.Name(), Size: info.Size(), Time: info.ModTime()})
	}
	// 新的在前
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

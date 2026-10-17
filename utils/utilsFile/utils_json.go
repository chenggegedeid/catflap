package utilsFile

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// // 读取config 配置文件
// func ReadConfig[T any](config *T, configPath string, createConfig func() (config T, err error)) (err error) {
// 	//检测文件是否存在
// 	if fileInfo, err := os.Stat(configPath); os.IsNotExist(err) || fileInfo.Size() == 0 {
// 		//不存在创建空配置文件
// 		*config, err = createConfig()
// 		return err

// 	} else {
// 		//文件存在读取配置文件
// 		*config, err = ReadJsonFile[T](configPath)
// 		if err != nil {
// 			logrus.Error("配置文件读取失败：", err)
// 			return err
// 		}

// 	}
// 	return nil

// }

// func CreateConfig[T any](config T, configPath string) (T, error) {
// 	// 确保 config 目录存在
// 	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
// 		logrus.Error("创建目录失败：", err)
// 		return config, err
// 	}

// 	// 写入一个空的配置文件
// 	if err := WriteJsonFile(configPath, config); err != nil {
// 		logrus.Error("配置文件写入失败：", err)
// 		return config, err
// 	}
// 	return config, nil
// }

// // UpdateConfig 更新配置文件
// func UpdateConfig(config any, configPath string) error {
// 	if err := WriteJsonFile(configPath, config); err != nil {
// 		logrus.Error("配置文件写入失败：", err)
// 		return err
// 	}
// 	return nil
// }

// 读取json配置文件
func ReadJsonFile[T any](filePath string) (T, error) {

	var result T

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return result, err
	}

	// 反序列化
	if err = json.Unmarshal(data, &result); err != nil {
		return result, err
	}

	return result, nil
}

// 写入json配置文件
func WriteJsonFile[T any](filePath string, obj T) error {
	// 序列化为json，带缩进
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}

	/**
	 * **先写临时文件，再原子改名**。
	 *
	 * 直接 WriteFile 的风险：写到一半进程被杀（断电、kill -9、
	 * 容器被回收），目标文件就剩半截 JSON —— 下次启动读不出来，
	 * 整个导航页起不来，而用户只是改了个图标。
	 *
	 * rename 在同一文件系统内是原子的：要么还是旧的完整配置，
	 * 要么是新的完整配置，不存在中间态。
	 */
	dir := filepath.Dir(filePath)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		// 退回到直接写：至少别因为连临时文件都建不了就彻底失败
		return writeDirect(filePath, data)
	}
	tmpName := tmp.Name()

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()

	if err = os.Chmod(tmpName, PrivateFileMode); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err = os.Rename(tmpName, filePath); err != nil {
		os.Remove(tmpName)
		return err
	}

	// 已存在的旧文件权限不会被 rename 改掉（用的是临时文件的权限 —— 已设过）
	return nil
}

// writeDirect 兜底：临时文件都建不了时的最后退路
func writeDirect(filePath string, data []byte) error {
	if err := os.WriteFile(filePath, data, PrivateFileMode); err != nil {
		return err
	}
	return os.Chmod(filePath, PrivateFileMode)
}

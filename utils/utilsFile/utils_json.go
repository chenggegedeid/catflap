package utilsFile

import (
	"encoding/json"
	"os"
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

	// 配置文件里有 JWT 密钥、Cloudflare Token、DDNS 密钥，只给属主读写
	if err = os.WriteFile(filePath, data, PrivateFileMode); err != nil {
		return err
	}

	// WriteFile 只在新建文件时应用权限，已存在的旧文件权限不变。
	// 显式收一次，让历史版本留下的 0644 在下次保存时被纠正。
	if err = os.Chmod(filePath, PrivateFileMode); err != nil {
		return err
	}

	return nil
}

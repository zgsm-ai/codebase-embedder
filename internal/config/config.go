package config

import (
	"errors"

	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	Auth struct {
		UserInfoHeader string
	}
	Database    Database
	Redis       RedisConfig
	IndexTask   IndexTaskConf
	VectorStore VectorStoreConf
	Cleaner     CleanerConf
	Validation  ValidationConfig
	TokenLimit  TokenLimitConf
}

// TokenLimitConf token限流配置
type TokenLimitConf struct {
	MaxRunningTasks int  `json:"max_running_tasks" yaml:"max_running_tasks"`
	Enabled         bool `json:"enabled" yaml:"enabled"`
}

// Validate 实现 Validator 接口
func (c Config) Validate() error {
	if len(c.Name) == 0 {
		return errors.New("name 不能为空")
	}
	return nil
}

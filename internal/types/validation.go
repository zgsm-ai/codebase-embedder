package types

import (
	"time"
)

// ValidationStatus 验证状态
type ValidationStatus string

const (
	ValidationStatusSuccess ValidationStatus = "success"
	ValidationStatusFailed  ValidationStatus = "failed"
	ValidationStatusPartial ValidationStatus = "partial"
	ValidationStatusSkipped ValidationStatus = "skipped"
)

// FileStatus 文件状态
type FileStatus string

const (
	FileStatusMatched    FileStatus = "matched"
	FileStatusMismatched FileStatus = "mismatched"
	FileStatusMissing    FileStatus = "missing"
	FileStatusSkipped    FileStatus = "skipped"
)

// ValidationResult 验证结果
type ValidationResult struct {
	TotalFiles      int                `json:"total_files"`
	MatchedFiles    int                `json:"matched_files"`
	MismatchedFiles int                `json:"mismatched_files"`
	SkippedFiles    int                `json:"skipped_files"`
	Details         []ValidationDetail `json:"details"`
	Status          ValidationStatus   `json:"status"`
	Timestamp       time.Time          `json:"timestamp"`
}

// ValidationDetail 单个文件验证详情
type ValidationDetail struct {
	FilePath string     `json:"file_path"`
	Status   FileStatus `json:"status"`
	Expected string     `json:"expected"` // 元数据中的状态
	Actual   string     `json:"actual"`   // 实际状态
	Error    string     `json:"error,omitempty"`
}

// SyncMetadata 同步元数据结构
type SyncMetadata struct {
	ClientId      string                 `json:"clientId"`
	CodebasePath  string                 `json:"codebasePath"`
	CodebaseName  string                 `json:"codebaseName"`
	ExtraMetadata map[string]interface{} `json:"extraMetadata"`
	FileList      map[string]string      `json:"fileList"` // 文件路径 -> 状态
	Timestamp     int64                  `json:"timestamp"`
}

// FileStats 文件统计信息
type FileStats struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// ValidationParams 验证参数
type ValidationParams struct {
	MetadataPath string            `json:"metadata_path"` // 元数据文件路径
	ExtractPath  string            `json:"extract_path"`  // 解压文件路径
	SkipPatterns []string          `json:"skip_patterns"` // 跳过文件模式
	Config       *ValidationConfig `json:"config"`        // 验证配置
}

// ValidationConfig 验证配置
type ValidationConfig struct {
	CheckContent   bool     `json:"check_content"`    // 是否检查文件内容
	FailOnMismatch bool     `json:"fail_on_mismatch"` // 不匹配时是否失败
	LogLevel       string   `json:"log_level"`        // 日志级别
	MaxConcurrency int      `json:"max_concurrency"`  // 最大并发数
	Enabled        bool     `json:"enabled"`          // 是否启用文件验证
	SkipPatterns   []string `json:"skip_patterns"`    // 跳过文件模式
}

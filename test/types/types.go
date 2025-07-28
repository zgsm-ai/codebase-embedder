package types

import "time"

// IndexStatus 表示索引状态
type IndexStatus struct {
	ClientId     string    `json:"clientId"`
	CodebasePath string    `json:"codebasePath"`
	Status       string    `json:"status"`
	Progress     float64   `json:"progress"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// IndexTask 表示索引任务
type IndexTask struct {
	Id           string    `json:"id"`
	ClientId     string    `json:"clientId"`
	CodebasePath string    `json:"codebasePath"`
	Status       string    `json:"status"`
	Progress     float64   `json:"progress"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// IndexSummary 表示索引摘要
type IndexSummary struct {
	ClientId     string    `json:"clientId"`
	CodebasePath string    `json:"codebasePath"`
	TotalFiles   int       `json:"totalFiles"`
	IndexedFiles int       `json:"indexedFiles"`
	TotalSize    int64     `json:"totalSize"`
	LastIndexed  time.Time `json:"lastIndexed"`
}

// Embedding 表示嵌入向量
type Embedding struct {
	Id           string    `json:"id"`
	ClientId     string    `json:"clientId"`
	CodebasePath string    `json:"codebasePath"`
	FilePath     string    `json:"filePath"`
	Content      string    `json:"content"`
	Vector       []float32 `json:"vector"`
	TokenCount   int       `json:"tokenCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ClientStats 表示客户端统计信息
type ClientStats struct {
	ClientId      string    `json:"clientId"`
	TotalIndices  int       `json:"totalIndices"`
	TotalFiles    int       `json:"totalFiles"`
	TotalSize     int64     `json:"totalSize"`
	LastActivity  time.Time `json:"lastActivity"`
}

// FileMetadata 表示文件元数据
type FileMetadata struct {
	ClientId     string    `json:"clientId"`
	CodebasePath string    `json:"codebasePath"`
	FilePath     string    `json:"filePath"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modifiedAt"`
	Hash         string    `json:"hash"`
}

// IndexingStats 表示索引统计
type IndexingStats struct {
	TaskId         string    `json:"taskId"`
	TotalFiles     int       `json:"totalFiles"`
	ProcessedFiles int       `json:"processedFiles"`
	FailedFiles    int       `json:"failedFiles"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
}
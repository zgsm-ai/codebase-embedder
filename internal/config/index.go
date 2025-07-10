package config

import "time"

type IndexTaskConf struct {
	Topic             string
	ConsumerGroup     string `json:",default=codebase_embedder"` // 消费者组名称（用于Streams）
	PoolSize          int
	QueueSize         int
	LockTimeout       time.Duration `json:",default=300s"`
	EmbeddingTask     EmbeddingTaskConf
	GraphTask         GraphTaskConf
	MsgMaxFailedTimes int `json:",default=3"`
}

type EmbeddingTaskConf struct {
	MaxConcurrency int  `json:",default=5"`
	Enabled        bool `json:",default=true"`
	Timeout        time.Duration
	// 滑动窗口重叠token数
	OverlapTokens     int
	MaxTokensPerChunk int
}

type GraphTaskConf struct {
	MaxConcurrency int  `json:",default=5"`
	Enabled        bool `json:",default=true"`
	Timeout        time.Duration
	ConfFile       string `json:",default=etc/codegraph.yaml"`
}

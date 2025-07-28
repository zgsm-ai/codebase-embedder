package types

// CreateEmbeddingRequest 创建嵌入任务请求
type CreateEmbeddingRequest struct {
	CodebasePath string `json:"codebasePath"`
	ClientID     string `json:"clientId"`
	ForceRebuild bool   `json:"forceRebuild"`
}

// CreateEmbeddingResponse 创建嵌入任务响应
type CreateEmbeddingResponse struct {
	TaskID string `json:"taskId"`
	Status string `json:"status"`
}

// DeleteEmbeddingRequest 删除嵌入任务请求
type DeleteEmbeddingRequest struct {
	CodebasePath string `json:"codebasePath"`
	ClientID     string `json:"clientId"`
}

// DeleteEmbeddingResponse 删除嵌入任务响应
type DeleteEmbeddingResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SemanticSearchRequest 语义搜索请求
type SemanticSearchRequest struct {
	ClientID     string `json:"clientId"`
	CodebasePath string `json:"codebasePath"`
	Query        string `json:"query"`
	Limit        int    `json:"limit"`
}

// SemanticSearchResponse 语义搜索响应
type SemanticSearchResponse struct {
	Results []SearchResult `json:"results"`
}

// SearchResult 搜索结果
type SearchResult struct {
	FilePath string  `json:"filePath"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
}

// SummaryRequest 摘要请求
type SummaryRequest struct {
	ClientID     string `json:"clientId"`
	CodebasePath string `json:"codebasePath"`
}

// SummaryResponse 摘要响应
type SummaryResponse struct {
	TotalFiles   int    `json:"totalFiles"`
	IndexedFiles int    `json:"indexedFiles"`
	TotalSize    int64  `json:"totalSize"`
	LastIndexed  string `json:"lastIndexed"`
}
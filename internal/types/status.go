package types

// FileStatusRequest 文件状态查询请求
type FileStatusRequest struct {
	ClientId     string `json:"clientId"`                        // 客户ID，machineID
	CodebasePath string `json:"codebasePath"`                    // 项目绝对路径（按照操作系统格式）
	CodebaseName string `json:"codebaseName"`                    // 项目名称
	ChunkNumber  int    `json:"chunkNumber,optional,default=0"`  // 当前分片
	TotalChunks  int    `json:"totalChunks,optional,default=1"`  // 分片总数，当代码过大时候采用分片上传（默认为1）
}

// FileStatusResponseData 文件状态查询响应数据
type FileStatusResponseData struct {
	Status      string `json:"status"`      // 处理状态: pending, processing, completed, failed
	Progress    int    `json:"progress"`    // 处理进度百分比 0-100
	TotalFiles  int    `json:"totalFiles"`  // 总文件数
	Processed   int    `json:"processed"`   // 已处理文件数
	Failed      int    `json:"failed"`      // 失败文件数
	Message     string `json:"message"`     // 状态描述信息
	UpdatedAt   string `json:"updatedAt"`   // 最后更新时间
	TaskId      int    `json:"taskId"`      // 任务ID
	ChunkNumber int    `json:"chunkNumber"` // 当前分片
	TotalChunks int    `json:"totalChunks"` // 分片总数
}
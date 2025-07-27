package logic

import (
	"context"
	"fmt"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/dao/query"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// StatusLogic 文件状态查询逻辑
type StatusLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewStatusLogic 创建文件状态查询逻辑
func NewStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StatusLogic {
	return &StatusLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetFileStatus 获取文件处理状态
func (l *StatusLogic) GetFileStatus(req *types.FileStatusRequest) (*types.FileStatusResponseData, error) {
	// 首先从Redis获取状态
	statusManager := l.svcCtx.StatusManager
	redisStatus, err := statusManager.GetFileStatus(l.ctx, req.ClientId, req.CodebasePath, req.CodebaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get status from redis: %w", err)
	}
	
	// 如果Redis中有状态，直接返回
	if redisStatus != nil {
		// 更新分片信息
		redisStatus.ChunkNumber = req.ChunkNumber
		redisStatus.TotalChunks = req.TotalChunks
		return redisStatus, nil
	}
	
	// Redis中没有状态，从数据库查询
	q := query.Use(nil) // 使用默认查询实例
	indexHistory := q.IndexHistory
	history, err := indexHistory.WithContext(l.ctx).
		Where(indexHistory.CodebasePath.Eq(req.CodebasePath)).
		Where(indexHistory.CodebaseName.Eq(req.CodebaseName)).
		Order(indexHistory.CreatedAt.Desc()).
		First()
	
	if err != nil {
		// 如果没有找到记录，返回初始状态
		return &types.FileStatusResponseData{
			Status:      "pending",
			Progress:    0,
			TotalFiles:  0,
			Processed:   0,
			Failed:      0,
			Message:     "等待处理",
			UpdatedAt:   time.Now().Format("2006-01-02 15:04:05"),
			TaskId:      0,
			ChunkNumber: req.ChunkNumber,
			TotalChunks: req.TotalChunks,
		}, nil
	}

	// 根据历史记录计算状态
	status := l.convertStatus(history.Status)
	
	// 处理可能为nil的指针字段
	totalFiles := int32(0)
	if history.TotalFileCount != nil {
		totalFiles = *history.TotalFileCount
	}
	
	processedFiles := int32(0)
	if history.TotalSuccessCount != nil {
		processedFiles = *history.TotalSuccessCount
	}
	
	failedFiles := int32(0)
	if history.TotalFailCount != nil {
		failedFiles = *history.TotalFailCount
	}
	
	progress := l.calculateProgress(status, int(processedFiles), int(totalFiles))
	
	response := &types.FileStatusResponseData{
		Status:      status,
		Progress:    progress,
		TotalFiles:  int(totalFiles),
		Processed:   int(processedFiles),
		Failed:      int(failedFiles),
		Message:     l.getStatusMessage(status),
		UpdatedAt:   history.UpdatedAt.Format("2006-01-02 15:04:05"),
		TaskId:      int(history.ID),
		ChunkNumber: req.ChunkNumber,
		TotalChunks: req.TotalChunks,
	}
	
	// 将状态缓存到Redis
	_ = statusManager.SetFileStatus(l.ctx, req.ClientId, req.CodebasePath, req.CodebaseName, response)
	
	return response, nil
}

// convertStatus 转换数据库状态为API状态
func (l *StatusLogic) convertStatus(dbStatus string) string {
	switch dbStatus {
	case "pending":
		return "pending"
	case "processing":
		return "processing"
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	default:
		return "pending"
	}
}

// calculateProgress 计算处理进度
func (l *StatusLogic) calculateProgress(status string, processed, total int) int {
	if total == 0 {
		return 0
	}
	
	switch status {
	case "completed":
		return 100
	case "failed":
		return 0
	case "processing":
		if total > 0 {
			return int(float64(processed) / float64(total) * 100)
		}
		return 0
	default:
		return 0
	}
}

// getStatusMessage 获取状态描述信息
func (l *StatusLogic) getStatusMessage(status string) string {
	switch status {
	case "pending":
		return "等待处理"
	case "processing":
		return "处理中"
	case "completed":
		return "处理完成"
	case "failed":
		return "处理失败"
	default:
		return "未知状态"
	}
}
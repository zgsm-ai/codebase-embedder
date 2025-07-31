package logic

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
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

	// 使用用户通过接口传入的SyncId作为键查询状态
	requestId := req.SyncId

	redisStatus, err := statusManager.GetFileStatus(l.ctx, requestId)
	if err != nil {
		return nil, fmt.Errorf("failed to get status from redis: %w", err)
	}

	logx.Infof("StatusLogic GetFileStatus: %+v", redisStatus)

	// 如果Redis中有状态，直接返回
	if redisStatus != nil {
		return redisStatus, nil
	}

	// 如果没有找到记录，返回初始状态
	return &types.FileStatusResponseData{
		Process:       "pending",
		TotalProgress: 0,
		FileList: []types.FileStatusItem{
			{Path: req.CodebasePath, Status: "pending"},
		},
	}, nil
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

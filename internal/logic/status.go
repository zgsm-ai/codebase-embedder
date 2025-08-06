package logic

import (
	"context"
	"fmt"

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

	// 如果Redis中有状态，直接返回
	if redisStatus != nil {
		return redisStatus, nil
	}

	// 如果没有找到记录，返回错误并说明原因
	return nil, fmt.Errorf("file status not found for request ID: %s, please ensure the file processing has been initiated", requestId)
}

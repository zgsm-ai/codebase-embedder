package logic

import (
	"context"
	"errors"
	"fmt"
	"github.com/zgsm-ai/codebase-indexer/internal/dao/model"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"sync"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"gorm.io/gorm"

	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SummaryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSummaryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SummaryLogic {
	return &SummaryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SummaryLogic) Summary(req *types.IndexSummaryRequest) (*types.IndexSummaryResonseData, error) {
	clientId := req.ClientId
	clientPath := req.CodebasePath

	// 查找代码库记录
	codebase, err := l.svcCtx.Querier.Codebase.FindByClientIdAndPath(l.ctx, clientId, clientPath)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NewRecordNotFoundErr(types.NameCodeBase, fmt.Sprintf("client_id: %s, clientPath: %s", clientId, clientPath))
	}
	if err != nil {
		return nil, err
	}

	ctx := context.WithValue(l.ctx, tracer.Key, tracer.RequestTraceId(int(codebase.ID)))

	var (
		wg                 sync.WaitGroup
		embeddingSummary   *types.EmbeddingSummary
		codegraphSummary   *types.CodeGraphSummary
		embeddingIndexTask *model.IndexHistory
		codegraphIndexTask *model.IndexHistory
		lastSyncHistory    *model.SyncHistory
	)

	// 定义超时时间
	timeout := 5 * time.Second

	// 获取向量索引状态（带超时控制）
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel() // 避免资源泄漏

		var err error
		embeddingSummary, err = l.svcCtx.VectorStore.GetIndexSummary(timeoutCtx, codebase.ID, codebase.Path)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				tracer.WithTrace(ctx).Errorf("embedding summary query timed out after %v", timeoutCtx)
			} else {
				tracer.WithTrace(ctx).Errorf("failed to get embedding summary, err:%v", err)
			}
			return
		}
	}()

	// 获取最新的embedding索引任务（带超时控制）
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel() // 避免资源泄漏
		embeddingIndexTask, err = l.svcCtx.Querier.IndexHistory.GetLatestTaskHistory(timeoutCtx, codebase.ID, types.TaskTypeEmbedding)
		if err != nil {
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				tracer.WithTrace(timeoutCtx).Errorf("embedding index task query timed out after %v", timeout)
			} else {
				tracer.WithTrace(timeoutCtx).Errorf("failed to get latest embedding index task, err:%v", err)
			}
		}
	}()

	// 获取最新的codegraph索引任务（带超时控制）
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel() // 避免资源泄漏

		var err error
		codegraphIndexTask, err = l.svcCtx.Querier.IndexHistory.GetLatestTaskHistory(timeoutCtx, codebase.ID, types.TaskTypeCodegraph)
		if err != nil {
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				tracer.WithTrace(timeoutCtx).Errorf("codegraph index task query timed out after %v", timeout)
			} else {
				tracer.WithTrace(timeoutCtx).Errorf("failed to get latest codegraph index task, err:%v", err)
			}
		}
	}()
	// 获取最新的同步历史
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel() // 避免资源泄漏
		var err error
		lastSyncHistory, err = l.svcCtx.Querier.SyncHistory.FindLatest(timeoutCtx, codebase.ID)
		if err != nil {
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				tracer.WithTrace(timeoutCtx).Errorf("codegraph index task query timed out after %v", timeout)
			} else {
				tracer.WithTrace(timeoutCtx).Errorf("failed to get latest sync history, err:%v", err)
			}
		}
	}()

	// 等待所有协程完成
	wg.Wait()

	resp := &types.IndexSummaryResonseData{
		TotalFiles: int(codebase.FileCount),
		Embedding: types.EmbeddingSummary{
			Status: types.TaskStatusPending,
		},
		CodeGraph: types.CodeGraphSummary{
			Status: types.TaskStatusPending,
		},
	}

	if embeddingIndexTask != nil {
		resp.Embedding.Status = convertStatus(embeddingIndexTask.Status)
		resp.Embedding.LastIndexAt = embeddingIndexTask.UpdatedAt.Format("2006-01-02 15:04:05")
	} else if embeddingSummary.TotalChunks > 0 {
		resp.Embedding.Status = types.TaskStatusSuccess
	}

	if codegraphIndexTask != nil {
		resp.CodeGraph.Status = convertStatus(codegraphIndexTask.Status)
		resp.CodeGraph.LastIndexAt = codegraphIndexTask.UpdatedAt.Format("2006-01-02 15:04:05")
	} else if codegraphSummary.TotalFiles > 0 {
		resp.CodeGraph.Status = types.TaskStatusSuccess
	}
	if lastSyncHistory != nil {
		resp.LastSyncAt = lastSyncHistory.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	if embeddingSummary != nil {
		resp.Embedding.TotalChunks = embeddingSummary.TotalChunks
		resp.Embedding.TotalFiles = embeddingSummary.TotalFiles
	}
	if codegraphSummary != nil {
		resp.CodeGraph.TotalFiles = codegraphSummary.TotalFiles
	}

	return resp, nil
}

func convertStatus(status string) string {
	var embeddingStatus string
	switch status {
	case types.TaskStatusSuccess:
		embeddingStatus = types.TaskStatusSuccess
	case types.TaskStatusRunning:
		embeddingStatus = types.TaskStatusRunning
	case types.TaskStatusPending:
		embeddingStatus = types.TaskStatusPending
	default:
		embeddingStatus = types.TaskStatusFailed
	}
	return embeddingStatus
}

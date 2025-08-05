package job

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

type IndexTask struct {
	SvcCtx  *svc.ServiceContext
	LockMux *redsync.Mutex
	Params  *IndexTaskParams
}

type IndexTaskParams struct {
	SyncID       int32  // 同步操作ID
	CodebaseID   int32  // 代码库ID
	CodebasePath string // 代码库路径
	CodebaseName string // 代码库名字
	ClientId     string // 客户端ID
	RequestId    string // 请求ID，用于状态管理
	Files        map[string][]byte
	Metadata     *types.SyncMetadata // 同步元数据
}

func (i *IndexTask) Run(ctx context.Context) (embedTaskOk bool, graphTaskOk bool) {
	start := time.Now()
	tracer.WithTrace(ctx).Infof("index task started")

	// 解锁
	defer func() {
		if err := i.SvcCtx.DistLock.Unlock(ctx, i.LockMux); err != nil {
			tracer.WithTrace(ctx).Errorf("index task unlock failed, key %s, err:%v", i.LockMux.Name(), err)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2) // 两个待等待的任务

	var embedErr error

	// 启动嵌入任务
	go func() {
		defer wg.Done()
		embedErr = i.buildEmbedding(ctx)
		if embedErr != nil {
			tracer.WithTrace(ctx).Errorf("embedding task failed:%v", embedErr)
		}
	}()

	// 等待两个任务完成
	wg.Wait()

	embedTaskOk = embedErr == nil

	tracer.WithTrace(ctx).Infof("index task end, cost %d ms. embedding ok? %t, graph ok? %t",
		time.Since(start).Milliseconds(), embedTaskOk, graphTaskOk)
	return
}

func (i *IndexTask) buildEmbedding(ctx context.Context) error {

	for filePath, operation := range i.Params.Metadata.FileList {
		tracer.WithTrace(ctx).Infof("------------------------------------, %s :%s ms.", filePath, operation)
		// operations[filePath] = operation
	}

	fileOperations := make(map[string]string)
	if i.Params.Metadata != nil {
		fileOperations = extractFileOperations(i.Params.Metadata)
	}

	tracer.WithTrace(ctx).Infof("------------------------------------, %v ms.", fileOperations)

	// 状态修改为处理中
	_ = i.SvcCtx.StatusManager.UpdateFileStatus(ctx, i.Params.RequestId,
		func(status *types.FileStatusResponseData) {
			status.Process = "processing"
			status.TotalProgress = 0
			var fileStatusItems []types.FileStatusItem

			for path, _ := range i.Params.Files {
				fileStatusItem := types.FileStatusItem{
					Path:    path, // 使用当前处理的文件路径，而不是codebasePath
					Status:  "processing",
					Operate: fileOperations[path],
				}
				fileStatusItems = append(fileStatusItems, fileStatusItem)
			}

			status.FileList = fileStatusItems

		})

	start := time.Now()

	// 添加日志来跟踪参数
	tracer.WithTrace(ctx).Infof("DEBUG: index_task - i.Params.Files length: %d", len(i.Params.Files))
	tracer.WithTrace(ctx).Infof("DEBUG: index_task - i.Params.Metadata: %v", i.Params.Metadata)
	if i.Params.Metadata != nil {
		tracer.WithTrace(ctx).Infof("DEBUG: index_task - i.Params.Metadata.FileList length: %d", len(i.Params.Metadata.FileList))
	}

	embeddingTimeout, embeddingTimeoutCancel := context.WithTimeout(ctx, i.SvcCtx.Config.IndexTask.EmbeddingTask.Timeout)
	defer embeddingTimeoutCancel()
	eProcessor, err := NewEmbeddingProcessor(i.SvcCtx, i.Params)
	if err != nil {
		return fmt.Errorf("failed to create embedding task processor for message: %d, err: %w", i.Params.SyncID, err)
	}
	err = eProcessor.Process(embeddingTimeout)
	if err != nil {
		return fmt.Errorf("embedding task failed, err:%w", err)
	}
	tracer.WithTrace(ctx).Infof("embedding task end successfully, cost %d ms.", time.Since(start).Milliseconds())
	return nil
}

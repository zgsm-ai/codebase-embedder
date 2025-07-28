package job

import (
	"context"
	"fmt"
	"github.com/go-redsync/redsync/v4"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"sync"
	"time"
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
	Files        map[string][]byte
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

	start := time.Now()
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

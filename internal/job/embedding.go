package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"github.com/zgsm-ai/codebase-indexer/internal/parser"
	"github.com/zgsm-ai/codebase-indexer/internal/store/vector"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

type embeddingProcessor struct {
	baseProcessor
}

func NewEmbeddingProcessor(
	svcCtx *svc.ServiceContext,
	msg *IndexTaskParams,
) (Processor, error) {
	return &embeddingProcessor{
		baseProcessor: baseProcessor{
			svcCtx: svcCtx,
			params: msg,
		},
	}, nil
}

type fileProcessResult struct {
	chunks []*types.CodeChunk
	err    error
	path   string
}

func (t *embeddingProcessor) Process(ctx context.Context) error {
	tracer.WithTrace(ctx).Infof("start to execute embedding task, codebase: %s", t.params.CodebaseName)
	start := time.Now()

	err := func(t *embeddingProcessor) error {
		if err := t.initTaskHistory(ctx, types.TaskTypeEmbedding); err != nil {
			return err
		}

		t.totalFileCnt = int32(len(t.params.Files))
		var (
			addChunks       = make([]*types.CodeChunk, 0, t.totalFileCnt)
			deleteFilePaths = make(map[string]struct{})
			mu              sync.Mutex // 保护 addChunks
		)

		// 更新Redis中的处理状态为"processing"
		_ = t.svcCtx.StatusManager.UpdateFileStatus(ctx, t.params.ClientId, t.params.CodebasePath, t.params.CodebaseName,
			func(status *types.FileStatusResponseData) {
				status.Status = "processing"
				status.Message = "处理中"
				status.TotalFiles = int(t.totalFileCnt)
			})

		// 处理单个文件的函数
		processFile := func(path string, content []byte) error {
			var result fileProcessResult
			result.path = path

			select {
			case <-ctx.Done():
				return errs.RunTimeout
			default:
				chunks, err := t.splitFile(ctx, &types.SourceFile{Path: path, Content: content})
				if err != nil {
					if parser.IsNotSupportedFileError(err) {
						atomic.AddInt32(&t.ignoreFileCnt, 1)
						return nil
					}
					atomic.AddInt32(&t.failedFileCnt, 1)
					
					// 更新Redis中的失败状态
					_ = t.svcCtx.StatusManager.UpdateFileStatus(ctx, t.params.ClientId, t.params.CodebasePath, t.params.CodebaseName,
						func(status *types.FileStatusResponseData) {
							status.Failed = int(atomic.LoadInt32(&t.failedFileCnt))
							status.Processed = int(atomic.LoadInt32(&t.successFileCnt))
							status.Progress = int(float64(status.Processed) / float64(status.TotalFiles) * 100)
						})
					
					return err
				}
				mu.Lock()
				addChunks = append(addChunks, chunks...)
				mu.Unlock()
				atomic.AddInt32(&t.successFileCnt, 1)

				// 更新Redis中的成功状态
				_ = t.svcCtx.StatusManager.UpdateFileStatus(ctx, t.params.ClientId, t.params.CodebasePath, t.params.CodebaseName,
					func(status *types.FileStatusResponseData) {
						status.Processed = int(atomic.LoadInt32(&t.successFileCnt))
						status.Progress = int(float64(status.Processed) / float64(status.TotalFiles) * 100)
					})

			}
			return nil
		}

		// 使用基础结构的并发处理方法
		if err := t.processFilesConcurrently(ctx, processFile, t.svcCtx.Config.IndexTask.EmbeddingTask.MaxConcurrency); err != nil {
			return err
		}
		var saveErrs []error
		// 先删除，再写入
		if len(deleteFilePaths) > 0 {
			var deleteChunks []*types.CodeChunk
			for path := range deleteFilePaths {
				deleteChunks = append(deleteChunks, &types.CodeChunk{
					CodebaseId:   t.params.CodebaseID,
					CodebasePath: t.params.CodebasePath,
					CodebaseName: t.params.CodebaseName,
					FilePath:     path,
				})
			}
			err := t.svcCtx.VectorStore.DeleteCodeChunks(ctx, deleteChunks, vector.Options{
				CodebaseId:   t.params.CodebaseID,
				CodebasePath: t.params.CodebasePath,
				CodebaseName: t.params.CodebaseName,
				SyncId:       t.params.SyncID,
			})
			if err != nil {
				tracer.WithTrace(ctx).Errorf("embedding task delete code chunks failed: %v", err)
				t.failedFileCnt += int32(len(deleteFilePaths))
				saveErrs = append(saveErrs, err)
			}
		}

		// 批量处理结果
		if len(addChunks) > 0 {
			err := t.svcCtx.VectorStore.UpsertCodeChunks(ctx, addChunks, vector.Options{
				CodebaseId:   t.params.CodebaseID,
				CodebasePath: t.params.CodebasePath,
				CodebaseName: t.params.CodebaseName,
				SyncId:       t.params.SyncID,
			})
			if err != nil {
				tracer.WithTrace(ctx).Errorf("embedding task upsert code chunks failed: %v", err)
				t.failedFileCnt = t.successFileCnt
				t.successFileCnt = 0
				saveErrs = append(saveErrs, err)
			}
		}
		if len(saveErrs) > 0 {
			return errors.Join(saveErrs...)
		}
		// update task status
		if err := t.updateTaskSuccess(ctx); err != nil {
			tracer.WithTrace(ctx).Errorf("embedding task update status success error:%v", err)
		}

		// 更新Redis中的最终状态
		finalStatus := "completed"
		if t.failedFileCnt > 0 {
			finalStatus = "failed"
		}
		_ = t.svcCtx.StatusManager.UpdateFileStatus(ctx, t.params.ClientId, t.params.CodebasePath, t.params.CodebaseName,
			func(status *types.FileStatusResponseData) {
				status.Status = finalStatus
				status.Progress = 100
				status.Processed = int(atomic.LoadInt32(&t.successFileCnt))
				status.Failed = int(atomic.LoadInt32(&t.failedFileCnt))
				if finalStatus == "completed" {
					status.Message = "处理完成"
				} else {
					status.Message = "处理失败"
				}
			})

		return nil
	}(t)

	if t.handleIfTaskFailed(ctx, err) {
		return fmt.Errorf("embedding task failed to update status, err:%v", err)
	}

	tracer.WithTrace(ctx).Infof("embedding task end successfully, cost: %d ms, total: %d, success: %d, failed: %d",
		time.Since(start).Milliseconds(), t.totalFileCnt, t.successFileCnt, t.failedFileCnt)
	return nil
}

func (t *embeddingProcessor) splitFile(ctx context.Context, file *types.SourceFile) ([]*types.CodeChunk, error) {
	// 切分文件
	return t.svcCtx.CodeSplitter.Split(&types.SourceFile{
		CodebaseId:   t.params.CodebaseID,
		CodebasePath: t.params.CodebasePath,
		CodebaseName: t.params.CodebaseName,
		Path:         file.Path,
		Content:      file.Content,
	})
}

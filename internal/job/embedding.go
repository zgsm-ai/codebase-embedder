package job

import (
	"context"
	"errors"
	"fmt"
	"github.com/zgsm-ai/codebase-indexer/internal/parser"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"github.com/zgsm-ai/codebase-indexer/internal/store/vector"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
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
					return err
				}
				mu.Lock()
				addChunks = append(addChunks, chunks...)
				mu.Unlock()
				atomic.AddInt32(&t.successFileCnt, 1)

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
		return nil
	}(t)

	if t.handleIfTaskFailed(ctx, err) {
		return fmt.Errorf("embedding task failed to update status, err:%v", err)
	}

	tracer.WithTrace(ctx).Infof("embedding task end successfully, cost: %d ms, total: %d, success: %d, failed: %d,params:%+v",
		time.Since(start).Milliseconds(), t.totalFileCnt, t.successFileCnt, t.failedFileCnt, t.params)
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

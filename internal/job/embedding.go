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

// extractFileOperations 从同步元数据中提取文件操作类型
func extractFileOperations(metadata *types.SyncMetadata) map[string]string {
	operations := make(map[string]string)

	// 遍历FileList，该字段存储了文件路径到操作类型的映射
	for filePath, operation := range metadata.FileList {
		operations[filePath] = operation
	}

	return operations
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

		var fileStatusItems []types.FileStatusItem

		// 从参数中获取同步元数据
		var syncMetadata *types.SyncMetadata
		if t.params.Metadata != nil {
			syncMetadata = t.params.Metadata
		}

		// 提取文件操作类型
		fileOperations := make(map[string]string)
		if syncMetadata != nil {
			fileOperations = extractFileOperations(syncMetadata)
		}

		for path, _ := range t.params.Files {
			fileStatusItem := types.FileStatusItem{
				Path:    path, // 使用当前处理的文件路径，而不是codebasePath
				Status:  "processing",
				Operate: fileOperations[path], // 设置操作类型
			}
			fileStatusItems = append(fileStatusItems, fileStatusItem)
		}

		// 更新Redis中的处理状态为"processing"
		_ = t.svcCtx.StatusManager.UpdateFileStatus(ctx, t.params.RequestId,
			func(status *types.FileStatusResponseData) {
				status.Process = "processing"
				status.TotalProgress = 0
			})

		// 处理单个文件的函数
		processFile := func(path string, content []byte) error {
			var result fileProcessResult
			result.path = path

			select {
			case <-ctx.Done():
				return errs.RunTimeout
			default:
				tracer.WithTrace(ctx).Infof("execute embedding task, path: %s", path)
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
				t.failedFileCnt += t.successFileCnt
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
		// 更新最终状态
		_ = t.svcCtx.StatusManager.UpdateFileStatus(ctx, t.params.RequestId,
			func(status *types.FileStatusResponseData) {
				status.Process = finalStatus
				status.TotalProgress = 100
				status.FileList = fileStatusItems
				// 遍历 FileList，将所有状态为 "processing" 的文件标记为最终状态
				for i := range status.FileList {
					if status.FileList[i].Status == "processing" {
						status.FileList[i].Status = finalStatus
					}
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

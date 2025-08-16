package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zgsm-ai/codebase-indexer/internal/dao/model"
	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"github.com/zgsm-ai/codebase-indexer/internal/store/vector"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
	"gorm.io/gorm"
)

type UpdateEmbeddingLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateEmbeddingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateEmbeddingLogic {
	return &UpdateEmbeddingLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateEmbeddingLogic) UpdateEmbeddingPath(req *types.UpdateEmbeddingPathRequest) (resp *types.UpdateEmbeddingPathResponseData, err error) {
	clientId := req.ClientId
	codebasePath := req.CodebasePath
	oldPath := req.OldPath
	newPath := req.NewPath

	// 查找代码库记录
	codebase, err := l.svcCtx.Querier.Codebase.FindByClientIdAndPath(l.ctx, clientId, codebasePath)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NewRecordNotFoundErr(types.NameCodeBase, fmt.Sprintf("client_id: %s, codebasePath: %s", clientId, codebasePath))
	}
	if err != nil {
		return nil, err
	}

	// 检查是否是目录
	// fullOldPath := filepath.Join(codebasePath, oldPath)
	// info, err := os.Stat(fullOldPath)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to stat path %s: %w", fullOldPath, err)
	// }

	var modifiedFiles []string

	// if info.IsDir() {
	// 	// 处理目录情况
	// 	modifiedFiles, err = l.updateDirectoryPaths(codebase, oldPath, newPath)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to update directory paths: %w", err)
	// 	}
	// } else {
	// 	// 处理文件情况
	modifiedFiles, err = l.updateFilePath(codebase, oldPath, newPath)
	if err != nil {
		return nil, fmt.Errorf("failed to update file path: %w", err)
	}
	// }

	return &types.UpdateEmbeddingPathResponseData{
		ModifiedFiles: modifiedFiles,
		TotalFiles:    len(modifiedFiles),
	}, nil
}

func (l *UpdateEmbeddingLogic) updateDirectoryPaths(codebase *model.Codebase, oldDirPath, newDirPath string) ([]string, error) {
	// 获取该目录下所有的文件路径
	records, err := l.svcCtx.VectorStore.GetCodebaseRecords(l.ctx, codebase.ID, codebase.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get codebase records: %w", err)
	}

	var modifiedFiles []string
	var chunksToUpdate []*types.CodeChunk

	for _, record := range records {
		// 检查文件路径是否以旧目录路径开头
		if strings.HasPrefix(record.FilePath, oldDirPath) {
			// 构建新的文件路径
			newFilePath := strings.Replace(record.FilePath, oldDirPath, newDirPath, 1)

			// 创建需要更新的chunk
			chunk := &types.CodeChunk{
				CodebaseId:   codebase.ID,
				CodebasePath: codebase.Path,
				FilePath:     record.FilePath,
				Content:      []byte(record.Content),
				Language:     record.Language,
				Range:        record.Range,
				TokenCount:   record.TokenCount,
			}

			chunksToUpdate = append(chunksToUpdate, chunk)
			modifiedFiles = append(modifiedFiles, newFilePath)
		}
	}

	if len(chunksToUpdate) > 0 {
		// 先删除旧的chunks
		var chunksToDelete []*types.CodeChunk
		for _, chunk := range chunksToUpdate {
			chunksToDelete = append(chunksToDelete, &types.CodeChunk{
				CodebaseId:   chunk.CodebaseId,
				CodebasePath: chunk.CodebasePath,
				FilePath:     chunk.FilePath,
			})
		}

		err = l.svcCtx.VectorStore.DeleteCodeChunks(l.ctx, chunksToDelete, vector.Options{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete old chunks: %w", err)
		}

		// 更新文件路径
		for _, chunk := range chunksToUpdate {
			chunk.FilePath = strings.Replace(chunk.FilePath, oldDirPath, newDirPath, 1)
		}

		// 插入新的chunks
		err = l.svcCtx.VectorStore.InsertCodeChunks(l.ctx, chunksToUpdate, vector.Options{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to insert new chunks: %w", err)
		}
	}

	return modifiedFiles, nil
}

func (l *UpdateEmbeddingLogic) updateFilePath(codebase *model.Codebase, oldFilePath, newFilePath string) ([]string, error) {
	// 获取该文件的记录
	records, err := l.svcCtx.VectorStore.GetCodebaseRecords(l.ctx, codebase.ID, codebase.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get codebase records: %w", err)
	}

	var chunksToUpdate []*types.CodeChunk
	var modifiedFiles []string

	for _, record := range records {
		if record.FilePath == oldFilePath {
			// 创建需要更新的chunk
			chunk := &types.CodeChunk{
				CodebaseId:   codebase.ID,
				CodebasePath: codebase.Path,
				FilePath:     record.FilePath,
				Content:      []byte(record.Content),
				Language:     record.Language,
				Range:        record.Range,
				TokenCount:   record.TokenCount,
			}

			chunksToUpdate = append(chunksToUpdate, chunk)
			modifiedFiles = append(modifiedFiles, newFilePath)
		}
	}

	if len(chunksToUpdate) > 0 {
		// 先删除旧的chunks
		var chunksToDelete []*types.CodeChunk
		for _, chunk := range chunksToUpdate {
			chunksToDelete = append(chunksToDelete, &types.CodeChunk{
				CodebaseId:   chunk.CodebaseId,
				CodebasePath: chunk.CodebasePath,
				FilePath:     chunk.FilePath,
			})
		}

		err = l.svcCtx.VectorStore.DeleteCodeChunks(l.ctx, chunksToDelete, vector.Options{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete old chunks: %w", err)
		}

		// 更新文件路径
		for _, chunk := range chunksToUpdate {
			chunk.FilePath = newFilePath
		}

		// 插入新的chunks
		err = l.svcCtx.VectorStore.InsertCodeChunks(l.ctx, chunksToUpdate, vector.Options{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to insert new chunks: %w", err)
		}
	}

	return modifiedFiles, nil
}

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
	var pathUpdates []*types.CodeChunkPathUpdate

	for _, record := range records {
		// 检查文件路径是否以旧目录路径开头
		if strings.HasPrefix(record.FilePath, oldDirPath) {
			// 构建新的文件路径
			newFilePath := strings.Replace(record.FilePath, oldDirPath, newDirPath, 1)

			// 创建路径更新请求
			pathUpdate := &types.CodeChunkPathUpdate{
				CodebaseId:  codebase.ID,
				OldFilePath: record.FilePath,
				NewFilePath: newFilePath,
			}

			pathUpdates = append(pathUpdates, pathUpdate)
			modifiedFiles = append(modifiedFiles, newFilePath)
		}
	}

	if len(pathUpdates) > 0 {
		// 使用新的直接更新路径的方法，而不是删除再插入
		err = l.svcCtx.VectorStore.UpdateCodeChunksPaths(l.ctx, pathUpdates, vector.Options{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update chunk paths: %w", err)
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

	var modifiedFiles []string

	// 检查是否有需要更新的记录
	for _, record := range records {
		if record.FilePath == oldFilePath {
			modifiedFiles = append(modifiedFiles, newFilePath)
		}
	}

	if len(modifiedFiles) > 0 {
		// 使用直接更新路径的方法
		pathUpdates := []*types.CodeChunkPathUpdate{
			{
				CodebaseId:  codebase.ID,
				OldFilePath: oldFilePath,
				NewFilePath: newFilePath,
			},
		}

		err = l.svcCtx.VectorStore.UpdateCodeChunksPaths(l.ctx, pathUpdates, vector.Options{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update chunk paths: %w", err)
		}
	}

	return modifiedFiles, nil
}

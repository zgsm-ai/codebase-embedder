package logic

import (
	"context"
	"errors"
	"fmt"

	"github.com/zgsm-ai/codebase-indexer/internal/store/vector"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"

	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"gorm.io/gorm"

	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type IndexLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewIndexLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IndexLogic {
	return &IndexLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *IndexLogic) DeleteIndex(req *types.DeleteIndexRequest) (resp *types.DeleteIndexResponseData, err error) {
	clientId := req.ClientId
	clientPath := req.CodebasePath
	filePaths := req.FilePaths

	// 查找代码库记录
	codebase, err := l.svcCtx.Querier.Codebase.FindByClientIdAndPath(l.ctx, clientId, clientPath)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NewRecordNotFoundErr(types.NameCodeBase, fmt.Sprintf("client_id: %s, clientPath: %s", clientId, clientPath))
	}
	if err != nil {
		return nil, err
	}

	ctx := context.WithValue(l.ctx, tracer.Key, tracer.RequestTraceId(int(codebase.ID)))

	// 如果filePaths为空，则删除整个工程的嵌入数据
	if filePaths == "" {
		if err = l.svcCtx.VectorStore.DeleteByCodebase(ctx, codebase.ID, codebase.Path); err != nil {
			return nil, fmt.Errorf("failed to delete embedding codebase, err:%w", err)
		}
		return &types.DeleteIndexResponseData{}, nil
	}

	if err = l.svcCtx.VectorStore.DeleteDictionary(ctx, filePaths, vector.Options{CodebaseId: codebase.ID,
		CodebasePath: codebase.Path}); err != nil {
		return nil, fmt.Errorf("failed to delete embedding index, err:%w", err)
	}

	return &types.DeleteIndexResponseData{}, nil
}

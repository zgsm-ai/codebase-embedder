package vector

import (
	"context"
	"errors"
	"fmt"

	"github.com/zgsm-ai/codebase-indexer/internal/config"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// Store 向量存储接口
type Store interface {
	DeleteByCodebase(ctx context.Context, codebaseId int32, codebasePath string) error
	GetIndexSummary(ctx context.Context, codebaseId int32, codebasePath string) (*types.EmbeddingSummary, error)
	GetCodebaseRecords(ctx context.Context, codebaseId int32, codebasePath string) ([]*types.CodebaseRecord, error)
	InsertCodeChunks(ctx context.Context, docs []*types.CodeChunk, options Options) error
	UpsertCodeChunks(ctx context.Context, chunks []*types.CodeChunk, options Options) error
	DeleteCodeChunks(ctx context.Context, chunks []*types.CodeChunk, options Options) error
	Query(ctx context.Context, query string, topK int, options Options) ([]*types.SemanticFileItem, error)
	Close()
}

const vectorWeaviate = "weaviate"

type Options struct {
	CodebaseId   int32
	SyncId       int32
	CodebasePath string
	CodebaseName string
}

func NewVectorStore(cfg config.VectorStoreConf, embedder Embedder, reranker Reranker) (Store, error) {
	var vectorStoreImpl Store
	var err error
	switch cfg.Type {
	case vectorWeaviate:
		if cfg.Weaviate.Endpoint == types.EmptyString {
			return nil, errors.New("vector conf weaviate is required for weaviate type")
		}
		vectorStoreImpl, err = New(cfg, embedder, reranker)
	default:
		err = fmt.Errorf("unsupported vector type: %s", cfg.Type)
	}

	if err != nil {
		return nil, err
	}
	return vectorStoreImpl, nil
}

package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/internal/store/vector"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// MockVectorStoreInterface 是vector.Store接口的mock实现
type MockVectorStoreInterface struct {
	mock.Mock
}

// DeleteByCodebase 删除指定代码库的向量
func (m *MockVectorStoreInterface) DeleteByCodebase(ctx context.Context, codebaseId int32, codebasePath string) error {
	args := m.Called(ctx, codebaseId, codebasePath)
	return args.Error(0)
}

// GetIndexSummary 获取索引摘要信息
func (m *MockVectorStoreInterface) GetIndexSummary(ctx context.Context, codebaseId int32, codebasePath string) (*types.EmbeddingSummary, error) {
	args := m.Called(ctx, codebaseId, codebasePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.EmbeddingSummary), args.Error(1)
}

// GetCodebaseRecords 获取代码库记录
func (m *MockVectorStoreInterface) GetCodebaseRecords(ctx context.Context, codebaseId int32, codebasePath string) ([]*types.CodebaseRecord, error) {
	args := m.Called(ctx, codebaseId, codebasePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.CodebaseRecord), args.Error(1)
}

// InsertCodeChunks 插入代码块
func (m *MockVectorStoreInterface) InsertCodeChunks(ctx context.Context, docs []*types.CodeChunk, options vector.Options) error {
	args := m.Called(ctx, docs, options)
	return args.Error(0)
}

// UpsertCodeChunks 更新或插入代码块
func (m *MockVectorStoreInterface) UpsertCodeChunks(ctx context.Context, chunks []*types.CodeChunk, options vector.Options) error {
	args := m.Called(ctx, chunks, options)
	return args.Error(0)
}

// DeleteCodeChunks 删除代码块
func (m *MockVectorStoreInterface) DeleteCodeChunks(ctx context.Context, chunks []*types.CodeChunk, options vector.Options) error {
	args := m.Called(ctx, chunks, options)
	return args.Error(0)
}

// Query 查询相似代码块
func (m *MockVectorStoreInterface) Query(ctx context.Context, query string, topK int, options vector.Options) ([]*types.SemanticFileItem, error) {
	args := m.Called(ctx, query, topK, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.SemanticFileItem), args.Error(1)
}

// Close 关闭连接
func (m *MockVectorStoreInterface) Close() {
	m.Called()
}

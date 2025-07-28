package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/test/types"
)

// MockVectorStore 是向量存储接口的mock实现
type MockVectorStore struct {
	mock.Mock
}

// Store 存储嵌入向量
func (m *MockVectorStore) Store(ctx context.Context, embedding *types.Embedding) error {
	args := m.Called(ctx, embedding)
	return args.Error(0)
}

// Search 搜索相似向量
func (m *MockVectorStore) Search(ctx context.Context, clientId, codebasePath string, vector []float32, limit int) ([]*types.Embedding, error) {
	args := m.Called(ctx, clientId, codebasePath, vector, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Embedding), args.Error(1)
}

// DeleteByCodebase 删除指定代码库的向量
func (m *MockVectorStore) DeleteByCodebase(ctx context.Context, clientId, codebasePath string) error {
	args := m.Called(ctx, clientId, codebasePath)
	return args.Error(0)
}

// HealthCheck 健康检查
func (m *MockVectorStore) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Close 关闭连接
func (m *MockVectorStore) Close() error {
	args := m.Called()
	return args.Error(0)
}
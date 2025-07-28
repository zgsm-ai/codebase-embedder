package mocks

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/test/types"
)

// MockDB 是数据库接口的mock实现
type MockDB struct {
	mock.Mock
}

// GetIndexStatus 获取索引状态
func (m *MockDB) GetIndexStatus(clientId, codebasePath string) (*types.IndexStatus, error) {
	args := m.Called(clientId, codebasePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.IndexStatus), args.Error(1)
}

// CreateIndexTask 创建索引任务
func (m *MockDB) CreateIndexTask(ctx context.Context, task *types.IndexTask) (string, error) {
	args := m.Called(ctx, task)
	return args.String(0), args.Error(1)
}

// UpdateTaskStatus 更新任务状态
func (m *MockDB) UpdateTaskStatus(taskId string, status string, progress float64) error {
	args := m.Called(taskId, status, progress)
	return args.Error(0)
}

// GetTaskStatus 获取任务状态
func (m *MockDB) GetTaskStatus(taskId string) (*types.IndexTask, error) {
	args := m.Called(taskId)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.IndexTask), args.Error(1)
}

// DeleteIndex 删除索引
func (m *MockDB) DeleteIndex(clientId, codebasePath string) error {
	args := m.Called(clientId, codebasePath)
	return args.Error(0)
}

// GetIndexSummary 获取索引摘要
func (m *MockDB) GetIndexSummary(clientId, codebasePath string) (*types.IndexSummary, error) {
	args := m.Called(clientId, codebasePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.IndexSummary), args.Error(1)
}

// SaveEmbedding 保存嵌入向量
func (m *MockDB) SaveEmbedding(embedding *types.Embedding) error {
	args := m.Called(embedding)
	return args.Error(0)
}

// GetEmbeddingsByFile 获取文件的嵌入向量
func (m *MockDB) GetEmbeddingsByFile(clientId, codebasePath, filePath string) ([]*types.Embedding, error) {
	args := m.Called(clientId, codebasePath, filePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Embedding), args.Error(1)
}

// GetAllEmbeddings 获取所有嵌入向量
func (m *MockDB) GetAllEmbeddings(clientId, codebasePath string) ([]*types.Embedding, error) {
	args := m.Called(clientId, codebasePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Embedding), args.Error(1)
}

// DeleteEmbeddingsByCodebase 删除代码库的所有嵌入向量
func (m *MockDB) DeleteEmbeddingsByCodebase(clientId, codebasePath string) error {
	args := m.Called(clientId, codebasePath)
	return args.Error(0)
}

// GetIndexingHistory 获取索引历史
func (m *MockDB) GetIndexingHistory(clientId string, limit int) ([]*types.IndexTask, error) {
	args := m.Called(clientId, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.IndexTask), args.Error(1)
}

// CleanExpiredIndices 清理过期索引
func (m *MockDB) CleanExpiredIndices(expiration time.Duration) (int64, error) {
	args := m.Called(expiration)
	return args.Get(0).(int64), args.Error(1)
}

// CountTotalIndices 统计总索引数
func (m *MockDB) CountTotalIndices() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(0)
}

// CountIndicesByClient 按客户端统计索引数
func (m *MockDB) CountIndicesByClient(clientId string) (int64, error) {
	args := m.Called(clientId)
	return args.Get(0).(int64), args.Error(0)
}

// GetClientStats 获取客户端统计信息
func (m *MockDB) GetClientStats(clientId string) (*types.ClientStats, error) {
	args := m.Called(clientId)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.ClientStats), args.Error(1)
}

// SaveFileMetadata 保存文件元数据
func (m *MockDB) SaveFileMetadata(metadata *types.FileMetadata) error {
	args := m.Called(metadata)
	return args.Error(0)
}

// GetFileMetadata 获取文件元数据
func (m *MockDB) GetFileMetadata(clientId, codebasePath, filePath string) (*types.FileMetadata, error) {
	args := m.Called(clientId, codebasePath, filePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.FileMetadata), args.Error(1)
}

// DeleteFileMetadata 删除文件元数据
func (m *MockDB) DeleteFileMetadata(clientId, codebasePath, filePath string) error {
	args := m.Called(clientId, codebasePath, filePath)
	return args.Error(0)
}

// BatchSaveEmbeddings 批量保存嵌入向量
func (m *MockDB) BatchSaveEmbeddings(embeddings []*types.Embedding) error {
	args := m.Called(embeddings)
	return args.Error(0)
}

// GetIndexingProgress 获取索引进度
func (m *MockDB) GetIndexingProgress(taskId string) (float64, error) {
	args := m.Called(taskId)
	return args.Get(0).(float64), args.Error(0)
}

// UpdateIndexingStats 更新索引统计
func (m *MockDB) UpdateIndexingStats(taskId string, stats *types.IndexingStats) error {
	args := m.Called(taskId, stats)
	return args.Error(0)
}

// GetIndexingStats 获取索引统计
func (m *MockDB) GetIndexingStats(taskId string) (*types.IndexingStats, error) {
	args := m.Called(taskId)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.IndexingStats), args.Error(1)
}

// Ping 检查数据库连接
func (m *MockDB) Ping() error {
	args := m.Called()
	return args.Error(0)
}

// Close 关闭数据库连接
func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// BeginTx 开始事务
func (m *MockDB) BeginTx() (interface{}, error) {
	args := m.Called()
	return args.Get(0), args.Error(1)
}

// CommitTx 提交事务
func (m *MockDB) CommitTx(tx interface{}) error {
	args := m.Called(tx)
	return args.Error(0)
}

// RollbackTx 回滚事务
func (m *MockDB) RollbackTx(tx interface{}) error {
	args := m.Called(tx)
	return args.Error(0)
}

// DeleteExistingIndex 删除现有索引
func (m *MockDB) DeleteExistingIndex(clientId, codebasePath string) error {
	args := m.Called(clientId, codebasePath)
	return args.Error(0)
}
package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// MockStatusManager 是StatusManager接口的mock实现
type MockStatusManager struct {
	mock.Mock
}

// GetFileStatus 从Redis获取文件处理状态
func (m *MockStatusManager) GetFileStatus(ctx context.Context, requestId string) (*types.FileStatusResponseData, error) {
	args := m.Called(ctx, requestId)
	first, _ := args.Get(0).(*types.FileStatusResponseData)
	return first, args.Error(1)
}

// SetFileStatusByRequestId 通过RequestId设置文件处理状态到Redis
func (m *MockStatusManager) SetFileStatusByRequestId(ctx context.Context, requestId string, status *types.FileStatusResponseData) error {
	args := m.Called(ctx, requestId, status)
	return args.Error(0)
}

func (m *MockStatusManager) UpdateFileStatus(ctx context.Context, requestId string, updateFn func(*types.FileStatusResponseData)) error {
	args := m.Called(ctx, requestId, updateFn)
	return args.Error(0)
}

// DeleteFileStatus 删除文件处理状态
func (m *MockStatusManager) DeleteFileStatus(ctx context.Context, clientID, codebasePath, codebaseName string) error {
	args := m.Called(ctx, clientID, codebasePath, codebaseName)
	return args.Error(0)
}

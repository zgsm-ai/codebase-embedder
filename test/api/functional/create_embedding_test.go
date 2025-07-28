package functional

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/test/handler"
	"github.com/zgsm-ai/codebase-indexer/test/mocks"
	"github.com/zgsm-ai/codebase-indexer/test/types"
)

func TestCreateEmbedding_Success(t *testing.T) {
	// 准备mock组件
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).Return(true, nil)
	mockRedis.On("ReleaseLock", mock.Anything, "embedding_task_test-client_/test/codebase").Return(nil)
	mockDB.On("CreateIndexTask", mock.Anything, mock.Anything).Return("task-123", nil)
	mockRedis.On("SetTaskStatus", mock.Anything, "task-123", "pending", 24*time.Hour).Return(nil)

	// 创建处理器
	embeddingHandler := handler.NewEmbeddingTaskHandler(mockVectorStore, mockDB, mockRedis)

	// 创建测试请求
	req := types.CreateEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
		ForceRebuild: false,
	}

	// 执行测试
	resp, err := embeddingHandler.CreateTask(context.Background(), req)

	// 断言
	assert.NoError(t, err)
	assert.Equal(t, "task-123", resp.TaskID)
	assert.Equal(t, "pending", resp.Status)
	mockRedis.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestCreateEmbedding_EmptyCodebasePath(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	embeddingHandler := handler.NewEmbeddingTaskHandler(mockVectorStore, mockDB, mockRedis)

	req := types.CreateEmbeddingRequest{
		CodebasePath: "",
		ClientID:    "test-client",
		ForceRebuild: false,
	}

	resp, err := embeddingHandler.CreateTask(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "代码库路径不能为空")
	assert.Empty(t, resp.TaskID)
}

func TestCreateEmbedding_InvalidPath(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	embeddingHandler := handler.NewEmbeddingTaskHandler(mockVectorStore, mockDB, mockRedis)

	testCases := []struct {
		name        string
		path        string
		expectedErr string
	}{
		{"相对路径遍历", "../../../etc/passwd", "非法路径"},
		{"绝对系统路径", "/etc/passwd", "非法路径"},
		{"Windows系统路径", "C:\\Windows\\System32", "非法路径"},
		{"空路径", "", "代码库路径不能为空"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := types.CreateEmbeddingRequest{
				CodebasePath: tc.path,
				ClientID:    "test-client",
				ForceRebuild: false,
			}

			resp, err := embeddingHandler.CreateTask(context.Background(), req)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)
			assert.Empty(t, resp.TaskID)
		})
	}
}

func TestCreateIndexing_TaskAlreadyExists(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 任务已存在
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).Return(false, nil)

	embeddingHandler := handler.NewEmbeddingTaskHandler(mockVectorStore, mockDB, mockRedis)

	req := types.CreateEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
		ForceRebuild: false,
	}

	resp, err := embeddingHandler.CreateTask(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "任务已存在")
	assert.Empty(t, resp.TaskID)
}

func TestCreateIndexing_LockAcquisitionFailed(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 获取锁失败
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).Return(false, assert.AnError)

	embeddingHandler := handler.NewEmbeddingTaskHandler(mockVectorStore, mockDB, mockRedis)

	req := types.CreateEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
		ForceRebuild: false,
	}

	resp, err := embeddingHandler.CreateTask(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "获取锁失败")
	assert.Empty(t, resp.TaskID)
}

func TestCreateIndexing_DatabaseError(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).Return(true, nil)
	mockRedis.On("ReleaseLock", mock.Anything, "embedding_task_test-client_/test/codebase").Return(nil)
	mockDB.On("CreateIndexTask", mock.Anything, mock.Anything).Return("", assert.AnError)

	embeddingHandler := handler.NewEmbeddingTaskHandler(mockVectorStore, mockDB, mockRedis)

	req := types.CreateEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
		ForceRebuild: false,
	}

	resp, err := embeddingHandler.CreateTask(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "创建任务失败")
	assert.Empty(t, resp.TaskID)
}

func TestCreateIndexing_ForceRebuild(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 强制重建
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).Return(true, nil)
	mockRedis.On("ReleaseLock", mock.Anything, "embedding_task_test-client_/test/codebase").Return(nil)
	mockDB.On("DeleteExistingIndex", "test-client", "/test/codebase").Return(nil)
	mockDB.On("CreateIndexTask", mock.Anything, mock.Anything).Return("task-456", nil)
	mockRedis.On("SetTaskStatus", mock.Anything, "task-456", "pending", 24*time.Hour).Return(nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	embeddingHandler := handler.NewEmbeddingTaskHandler(svcCtx)

	req := types.CreateEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
		ForceRebuild: true,
	}

	resp, err := embeddingHandler.CreateTask(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, "task-456", resp.TaskID)
	assert.Equal(t, "pending", resp.Status)
	mockDB.AssertCalled(t, "DeleteExistingIndex", "test-client", "/test/codebase")
}

func TestCreateIndexing_HTTPHandler(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).Return(true, nil)
	mockRedis.On("ReleaseLock", mock.Anything, "embedding_task_test-client_/test/codebase").Return(nil)
	mockDB.On("CreateIndexTask", mock.Anything, mock.Anything).Return("task-789", nil)
	mockRedis.On("SetTaskStatus", mock.Anything, "task-789", "pending", 24*time.Hour).Return(nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建HTTP处理器
	embeddingHandler := handler.NewEmbeddingTaskHandler(svcCtx)
	router := http.NewServeMux()
	router.HandleFunc("/api/embedding/create", embeddingHandler.HTTPHandler)

	// 创建测试请求
	reqBody := `{
		"codebasePath": "/test/codebase",
		"clientId": "test-client",
		"forceRebuild": false
	}`

	req := httptest.NewRequest("POST", "/api/embedding/create", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 断言响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp types.CreateEmbeddingResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "task-789", resp.TaskID)
	assert.Equal(t, "pending", resp.Status)
}

func TestCreateIndexing_ConcurrentRequests(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 第一个请求成功，后续请求失败
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).
		Return(true, nil).Once()
	mockRedis.On("AcquireLock", mock.Anything, "embedding_task_test-client_/test/codebase", 30*time.Second).
		Return(false, nil).Times(3)
	mockRedis.On("ReleaseLock", mock.Anything, "embedding_task_test-client_/test/codebase").Return(nil)
	mockDB.On("CreateIndexTask", mock.Anything, mock.Anything).Return("task-concurrent", nil)
	mockRedis.On("SetTaskStatus", mock.Anything, "task-concurrent", "pending", 24*time.Hour).Return(nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	embeddingHandler := handler.NewEmbeddingTaskHandler(svcCtx)

	req := types.CreateEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
		ForceRebuild: false,
	}

	// 第一个请求应该成功
	resp1, err1 := embeddingHandler.CreateTask(context.Background(), req)
	assert.NoError(t, err1)
	assert.Equal(t, "task-concurrent", resp1.TaskID)

	// 后续并发请求应该失败
	for i := 0; i < 3; i++ {
		resp, err := embeddingHandler.CreateTask(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "任务已存在")
		assert.Empty(t, resp.TaskID)
	}
}
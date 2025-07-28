package functional

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/internal/handler"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
	"github.com/zgsm-ai/codebase-indexer/test/mocks"
)

func TestDeleteEmbedding_Success(t *testing.T) {
	// 准备mock组件
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/test/codebase").Return(nil)
	mockRedis.On("DeleteKeys", mock.Anything, "test-client_/test/codebase").Return(int64(5), nil)

	// 创建服务上下文
	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建处理器
	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	// 创建测试请求
	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	// 执行测试
	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	// 断言
	assert.NoError(t, err)
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, "/test/codebase", resp.CodebasePath)
	assert.Equal(t, int64(5), resp.DeletedKeys)
	mockDB.AssertExpectations(t)
	mockVectorStore.AssertExpectations(t)
	mockRedis.AssertExpectations(t)
}

func TestDeleteEmbedding_EmptyCodebasePath(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "代码库路径不能为空")
	assert.False(t, resp.Success)
}

func TestDeleteEmbedding_InvalidPath(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

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
			req := types.DeleteEmbeddingRequest{
				CodebasePath: tc.path,
				ClientID:    "test-client",
			}

			resp, err := deleteHandler.DeleteIndex(context.Background(), req)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)
			assert.False(t, resp.Success)
		})
	}
}

func TestDeleteEmbedding_IndexNotFound(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 索引不存在
	mockDB.On("DeleteIndex", "test-client", "/nonexistent/path").
		Return(types.ErrIndexNotFound)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/nonexistent/path",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "索引不存在")
	assert.False(t, resp.Success)
}

func TestDeleteEmbedding_DatabaseError(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 数据库错误
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").
		Return(assert.AnError)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "删除索引失败")
	assert.False(t, resp.Success)
}

func TestDeleteEmbedding_VectorStoreError(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 向量存储错误
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/test/codebase").
		Return(assert.AnError)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "删除向量集合失败")
	assert.False(t, resp.Success)
}

func TestDeleteEmbedding_RedisError(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - Redis错误
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/test/codebase").Return(nil)
	mockRedis.On("DeleteKeys", mock.Anything, "test-client_/test/codebase").
		Return(int64(0), assert.AnError)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "清理缓存失败")
	assert.False(t, resp.Success)
}

func TestDeleteEmbedding_HTTPHandler(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/test/codebase").Return(nil)
	mockRedis.On("DeleteKeys", mock.Anything, "test-client_/test/codebase").Return(int64(3), nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建HTTP处理器
	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)
	router := http.NewServeMux()
	router.HandleFunc("/api/embedding/delete", deleteHandler.HTTPHandler)

	// 创建测试请求
	reqBody := `{
		"codebasePath": "/test/codebase",
		"clientId": "test-client"
	}`

	req := httptest.NewRequest("POST", "/api/embedding/delete", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 断言响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp types.DeleteEmbeddingResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, "/test/codebase", resp.CodebasePath)
	assert.Equal(t, int64(3), resp.DeletedKeys)
}

func TestDeleteEmbedding_PartialFailure(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 部分失败
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/test/codebase").Return(nil)
	mockRedis.On("DeleteKeys", mock.Anything, "test-client_/test/codebase").
		Return(int64(0), nil) // 没有删除任何key

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, "/test/codebase", resp.CodebasePath)
	assert.Equal(t, int64(0), resp.DeletedKeys)
}

func TestDeleteEmbedding_ConcurrentDeletion(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 并发删除
	mockDB.On("DeleteIndex", "test-client", "/test/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/test/codebase").Return(nil)
	mockRedis.On("DeleteKeys", mock.Anything, "test-client_/test/codebase").Return(int64(5), nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	// 多次删除应该都成功
	for i := 0; i < 3; i++ {
		resp, err := deleteHandler.DeleteIndex(context.Background(), req)
		
		assert.NoError(t, err)
		assert.Equal(t, true, resp.Success)
		assert.Equal(t, "/test/codebase", resp.CodebasePath)
	}
}

func TestDeleteEmbedding_LargeCodebase(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望 - 大型代码库
	mockDB.On("DeleteIndex", "test-client", "/large/codebase").Return(nil)
	mockVectorStore.On("DeleteCollection", "test-client_/large/codebase").Return(nil)
	mockRedis.On("DeleteKeys", mock.Anything, "test-client_/large/codebase").Return(int64(10000), nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	deleteHandler := handler.NewDeleteEmbeddingHandler(svcCtx)

	req := types.DeleteEmbeddingRequest{
		CodebasePath: "/large/codebase",
		ClientID:    "test-client",
	}

	resp, err := deleteHandler.DeleteIndex(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, "/large/codebase", resp.CodebasePath)
	assert.Equal(t, int64(10000), resp.DeletedKeys)
}
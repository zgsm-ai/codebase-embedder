package functional

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zgsm-ai/codebase-indexer/internal/handler"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
	"github.com/zgsm-ai/codebase-indexer/test/mocks"
)

func TestIndexSummary_Success(t *testing.T) {
	// 准备mock组件
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	expectedSummary := &types.IndexSummary{
		CodebasePath: "/test/codebase",
		TotalFiles:   150,
		IndexedFiles: 145,
		FailedFiles:  5,
		Status:       "completed",
		LastIndexed:  "2024-01-15T10:30:00Z",
		FileTypes: map[string]int{
			".go":  80,
			".js":  40,
			".py":  20,
			".md":  10,
		},
		TotalSize: 20485760, // 约20MB
	}

	mockDB.On("GetIndexSummary", "test-client", "/test/codebase").Return(expectedSummary, nil)

	// 创建服务上下文
	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建处理器
	summaryHandler := handler.NewSummaryHandler(svcCtx)

	// 创建测试请求
	req := types.SummaryRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	// 执行测试
	resp, err := summaryHandler.GetSummary(context.Background(), req)

	// 断言
	assert.NoError(t, err)
	assert.Equal(t, "/test/codebase", resp.CodebasePath)
	assert.Equal(t, 150, resp.TotalFiles)
	assert.Equal(t, 145, resp.IndexedFiles)
	assert.Equal(t, 5, resp.FailedFiles)
	assert.Equal(t, "completed", resp.Status)
	assert.Equal(t, 4, len(resp.FileTypes))
	assert.Equal(t, 20485760, resp.TotalSize)
	mockDB.AssertExpectations(t)
}

func TestIndexSummary_EmptyCodebasePath(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	summaryHandler := handler.NewSummaryHandler(svcCtx)

	req := types.SummaryRequest{
		CodebasePath: "",
		ClientID:    "test-client",
	}

	resp, err := summaryHandler.GetSummary(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "代码库路径不能为空")
	assert.Nil(t, resp)
}

func TestIndexSummary_NotFound(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock返回未找到
	mockDB.On("GetIndexSummary", "test-client", "/nonexistent/path").
		Return((*types.IndexSummary)(nil), types.ErrIndexNotFound)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	summaryHandler := handler.NewSummaryHandler(svcCtx)

	req := types.SummaryRequest{
		CodebasePath: "/nonexistent/path",
		ClientID:    "test-client",
	}

	resp, err := summaryHandler.GetSummary(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "索引不存在")
	assert.Nil(t, resp)
}

func TestIndexSummary_DatabaseError(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock返回数据库错误
	mockDB.On("GetIndexSummary", mock.Anything, mock.Anything).
		Return((*types.IndexSummary)(nil), assert.AnError)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	summaryHandler := handler.NewSummaryHandler(svcCtx)

	req := types.SummaryRequest{
		CodebasePath: "/test/codebase",
		ClientID:    "test-client",
	}

	resp, err := summaryHandler.GetSummary(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "获取索引摘要失败")
	assert.Nil(t, resp)
}

func TestIndexSummary_EmptyIndex(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置空索引的mock期望
	emptySummary := &types.IndexSummary{
		CodebasePath: "/test/empty",
		TotalFiles:   0,
		IndexedFiles: 0,
		FailedFiles:  0,
		Status:       "empty",
		FileTypes:    make(map[string]int),
		TotalSize:    0,
	}

	mockDB.On("GetIndexSummary", "test-client", "/test/empty").Return(emptySummary, nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	summaryHandler := handler.NewSummaryHandler(svcCtx)

	req := types.SummaryRequest{
		CodebasePath: "/test/empty",
		ClientID:    "test-client",
	}

	resp, err := summaryHandler.GetSummary(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, "/test/empty", resp.CodebasePath)
	assert.Equal(t, 0, resp.TotalFiles)
	assert.Equal(t, 0, resp.IndexedFiles)
	assert.Equal(t, "empty", resp.Status)
	assert.Empty(t, resp.FileTypes)
}

func TestIndexSummary_HTTPHandler(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	expectedSummary := &types.IndexSummary{
		CodebasePath: "/test/codebase",
		TotalFiles:   100,
		IndexedFiles: 95,
		FailedFiles:  5,
		Status:       "completed",
	}

	mockDB.On("GetIndexSummary", "test-client", "/test/codebase").Return(expectedSummary, nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建HTTP处理器
	summaryHandler := handler.NewSummaryHandler(svcCtx)
	router := http.NewServeMux()
	router.HandleFunc("/api/index/summary", summaryHandler.HTTPHandler)

	// 创建测试请求
	reqBody := `{
		"codebasePath": "/test/codebase",
		"clientId": "test-client"
	}`

	req := httptest.NewRequest("POST", "/api/index/summary", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 断言响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp types.SummaryResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "/test/codebase", resp.CodebasePath)
	assert.Equal(t, 100, resp.TotalFiles)
	assert.Equal(t, 95, resp.IndexedFiles)
}

func TestIndexSummary_LargeCodebase(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置大型代码库的mock期望
	largeSummary := &types.IndexSummary{
		CodebasePath: "/large/codebase",
		TotalFiles:   50000,
		IndexedFiles: 49950,
		FailedFiles:  50,
		Status:       "completed",
		LastIndexed:  "2024-01-15T10:30:00Z",
		FileTypes: map[string]int{
			".go":  20000,
			".js":  15000,
			".py":  10000,
			".java": 3000,
			".cpp": 2000,
			".md":   950,
		},
		TotalSize: 1073741824, // 1GB
	}

	mockDB.On("GetIndexSummary", "test-client", "/large/codebase").Return(largeSummary, nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	summaryHandler := handler.NewSummaryHandler(svcCtx)

	req := types.SummaryRequest{
		CodebasePath: "/large/codebase",
		ClientID:    "test-client",
	}

	resp, err := summaryHandler.GetSummary(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, 50000, resp.TotalFiles)
	assert.Equal(t, 49950, resp.IndexedFiles)
	assert.Equal(t, 50, resp.FailedFiles)
	assert.Equal(t, 6, len(resp.FileTypes))
	assert.Equal(t, 1073741824, resp.TotalSize)
}
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

func TestSemanticSearch_Success(t *testing.T) {
	// 准备mock组件
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	expectedResults := []*types.SearchResult{
		{
			FilePath:    "src/auth/user.go",
			Content:     "func AuthenticateUser(username, password string) bool",
			Score:       0.95,
			StartLine:   15,
			EndLine:     25,
			CodeSnippet: "func AuthenticateUser(username, password string) bool {\n    // 用户认证逻辑\n    return true\n}",
		},
	}
	
	mockVectorStore.On("Search", mock.Anything, "用户认证函数", 10).Return(expectedResults, nil)

	// 创建服务上下文
	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建处理器
	semanticHandler := handler.NewSemanticHandler(svcCtx)

	// 创建测试请求
	req := types.SemanticSearchRequest{
		Query:       "用户认证函数",
		CodebasePath: "/test/codebase",
		TopK:        10,
		ClientID:    "test-client",
	}

	// 执行测试
	resp, err := semanticHandler.Search(context.Background(), req)

	// 断言
	assert.NoError(t, err)
	assert.Equal(t, 1, len(resp.Results))
	assert.Equal(t, "src/auth/user.go", resp.Results[0].FilePath)
	assert.Equal(t, 0.95, resp.Results[0].Score)
	mockVectorStore.AssertExpectations(t)
}

func TestSemanticSearch_EmptyQuery(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	semanticHandler := handler.NewSemanticHandler(svcCtx)

	req := types.SemanticSearchRequest{
		Query:       "",
		CodebasePath: "/test/codebase",
		TopK:        10,
		ClientID:    "test-client",
	}

	resp, err := semanticHandler.Search(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "查询内容不能为空")
	assert.Nil(t, resp)
}

func TestSemanticSearch_InvalidTopK(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	semanticHandler := handler.NewSemanticHandler(svcCtx)

	testCases := []struct {
		name  string
		topK  int
		error string
	}{
		{"TopK为零", 0, "TopK必须大于0"},
		{"TopK为负数", -1, "TopK必须大于0"},
		{"TopK过大", 1001, "TopK不能超过1000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := types.SemanticSearchRequest{
				Query:       "测试查询",
				CodebasePath: "/test/codebase",
				TopK:        tc.topK,
				ClientID:    "test-client",
			}

			resp, err := semanticHandler.Search(context.Background(), req)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
			assert.Nil(t, resp)
		})
	}
}

func TestSemanticSearch_VectorStoreError(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock返回错误
	mockVectorStore.On("Search", mock.Anything, mock.Anything, mock.Anything).
		Return([]*types.SearchResult{}, assert.AnError)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	semanticHandler := handler.NewSemanticHandler(svcCtx)

	req := types.SemanticSearchRequest{
		Query:       "测试查询",
		CodebasePath: "/test/codebase",
		TopK:        10,
		ClientID:    "test-client",
	}

	resp, err := semanticHandler.Search(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "搜索失败")
	assert.Nil(t, resp)
}

func TestSemanticSearch_HTTPHandler(t *testing.T) {
	mockVectorStore := new(mocks.MockVectorStore)
	mockDB := new(mocks.MockDB)
	mockRedis := new(mocks.MockRedis)

	// 设置mock期望
	expectedResults := []*types.SearchResult{
		{
			FilePath: "src/main.go",
			Content:  "func main()",
			Score:    0.85,
		},
	}
	
	mockVectorStore.On("Search", mock.Anything, "main函数", 5).Return(expectedResults, nil)

	svcCtx := &svc.ServiceContext{
		VectorStore: mockVectorStore,
		DB:          mockDB,
		Redis:       mockRedis,
	}

	// 创建HTTP处理器
	semanticHandler := handler.NewSemanticHandler(svcCtx)
	router := http.NewServeMux()
	router.HandleFunc("/api/semantic/search", semanticHandler.HTTPHandler)

	// 创建测试请求
	reqBody := `{
		"query": "main函数",
		"codebasePath": "/test/codebase",
		"topK": 5,
		"clientId": "test-client"
	}`

	req := httptest.NewRequest("POST", "/api/semantic/search", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 断言响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp types.SemanticSearchResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(resp.Results))
	assert.Equal(t, "src/main.go", resp.Results[0].FilePath)
}
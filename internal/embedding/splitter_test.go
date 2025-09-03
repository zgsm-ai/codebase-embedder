package embedding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zgsm-ai/codebase-indexer/internal/parser"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

func TestSplitOpenAPIFile(t *testing.T) {
	// 创建测试用的 CodeSplitter
	splitOptions := SplitOptions{
		MaxTokensPerChunk:          1000,
		SlidingWindowOverlapTokens: 100,
		EnableMarkdownParsing:      true,
	}
	splitter, err := NewCodeSplitter(splitOptions)
	assert.NoError(t, err)

	// 定义测试文件
	testFiles := []struct {
		name        string
		filePath    string
		expectError bool
		expectCount int
		description string
	}{
		{
			name:        "OpenAPI 3.0 JSON 文件",
			filePath:    "/home/kcx/codeWorkspace/codebase-embedder/bin/openapi3.json",
			expectError: false,
			expectCount: 2, // /pets 和 /pets/{petId} 两个路径
			description: "应该成功分割 OpenAPI 3.0 JSON 文件",
		},
		{
			name:        "OpenAPI 3.0 YAML 文件",
			filePath:    "/home/kcx/codeWorkspace/codebase-embedder/bin/openapi3.yaml",
			expectError: false,
			expectCount: 2, // /users 和 /users/{id} 两个路径
			description: "应该成功分割 OpenAPI 3.0 YAML 文件",
		},
		{
			name:        "Swagger 2.0 JSON 文件",
			filePath:    "/home/kcx/codeWorkspace/codebase-embedder/bin/swagger2.json",
			expectError: false,
			expectCount: 14, // 14个不同的路径
			description: "应该成功分割 Swagger 2.0 JSON 文件",
		},
		{
			name:        "Swagger 2.0 YAML 文件",
			filePath:    "/home/kcx/codeWorkspace/codebase-embedder/bin/swagger2.yaml",
			expectError: true,  // 目前不支持Swagger 2.0 YAML 文件
			expectCount: 2, // /users 和 /users/{id} 两个路径
			description: "应该成功分割 Swagger 2.0 YAML 文件",
		},
	}

	for _, tt := range testFiles {
		t.Run(tt.name, func(t *testing.T) {
			// 读取文件内容
			content, err := os.ReadFile(tt.filePath)
			assert.NoError(t, err, "应该能够读取文件 %s", tt.filePath)

			// 创建测试用的 SourceFile
			sourceFile := &types.SourceFile{
				CodebaseId:   1,
				CodebasePath: "/test/path",
				CodebaseName: "test-codebase",
				Path:         filepath.Base(tt.filePath),
				Content:      content,
			}

			// 执行分割
			chunks, err := splitter.splitOpenAPIFile(sourceFile)

			// 验证结果
			if tt.expectError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, chunks, "错误时应该返回 nil chunks")
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, chunks, "成功时应该返回非 nil chunks")
				assert.Len(t, chunks, tt.expectCount, "应该返回正确数量的 chunks")

				// 验证每个 chunk 的基本属性
				for i, chunk := range chunks {
					assert.Equal(t, "doc", chunk.Language, "chunk %d 的语言应该是 'doc'", i)
					assert.Equal(t, sourceFile.CodebaseId, chunk.CodebaseId, "chunk %d 的 CodebaseId 应该匹配", i)
					assert.Equal(t, sourceFile.CodebasePath, chunk.CodebasePath, "chunk %d 的 CodebasePath 应该匹配", i)
					assert.Equal(t, sourceFile.CodebaseName, chunk.CodebaseName, "chunk %d 的 CodebaseName 应该匹配", i)
					assert.Equal(t, sourceFile.Path, chunk.FilePath, "chunk %d 的 FilePath 应该匹配", i)
					assert.Greater(t, chunk.TokenCount, 0, "chunk %d 的 TokenCount 应该大于 0", i)
					assert.NotEmpty(t, chunk.Content, "chunk %d 的 Content 不应该为空", i)

					// 验证分割后的文档是有效的 JSON
					var doc map[string]interface{}
					err := json.Unmarshal(chunk.Content, &doc)
					assert.NoError(t, err, "chunk %d 的内容应该是有效的 JSON", i)

					// 验证标题包含路径信息
					if info, exists := doc["info"]; exists {
						if infoMap, ok := info.(map[string]interface{}); ok {
							if title, exists := infoMap["title"]; exists {
								titleStr := title.(string)
								assert.Contains(t, titleStr, " - ", "chunk %d 的标题应该包含路径分隔符", i)
							}
						}
					}

					// 验证路径数量
					if paths, exists := doc["paths"]; exists {
						if pathsMap, ok := paths.(map[string]interface{}); ok {
							assert.Len(t, pathsMap, 1, "chunk %d 应该只包含一个路径", i)
						}
					}
				}
			}
		})
	}
}

func TestValidateOpenAPISpec(t *testing.T) {
	splitOptions := SplitOptions{
		MaxTokensPerChunk:          1000,
		SlidingWindowOverlapTokens: 100,
		EnableMarkdownParsing:      true,
	}
	splitter, err := NewCodeSplitter(splitOptions)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		content     []byte
		expectVer   APIVersion
		filePath    string
		expectError bool
	}{
		{
			name:        "OpenAPI 3.0 JSON",
			content:     []byte(`{"openapi": "3.0.3", "info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   OpenAPI3,
			filePath:    "test.json",
			expectError: false,
		},
		{
			name:        "Swagger 2.0 JSON",
			content:     []byte(`{"swagger": "2.0", "info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   Swagger2,
			filePath:    "test.yaml",
			expectError: false,
		},
		{
			name:        "无效 JSON",
			content:     []byte(`{ invalid json`),
			expectVer:   Unknown,
			filePath:    "test.json",
			expectError: true,
		},
		{
			name:        "不支持的版本",
			content:     []byte(`{"openapi": "4.0.0", "info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   Unknown,
			filePath:    "test.yaml",
			expectError: true,
		},
		{
			name:        "缺少版本字段",
			content:     []byte(`{"info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   Unknown,
			filePath:    "test.json",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := splitter.validateOpenAPISpec(tt.content, tt.filePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectVer, version)
		})
	}
}

// 测试边界情况
func TestSplitOpenAPIFileEdgeCases(t *testing.T) {
	splitOptions := SplitOptions{
		MaxTokensPerChunk:          1000,
		SlidingWindowOverlapTokens: 100,
		EnableMarkdownParsing:      true,
	}
	splitter, err := NewCodeSplitter(splitOptions)
	assert.NoError(t, err)

	t.Run("空路径的 OpenAPI 文档", func(t *testing.T) {
		doc := map[string]interface{}{
			"openapi": "3.0.0",
			"info": map[string]interface{}{
				"title":   "Test API",
				"version": "1.0.0",
			},
			"paths": map[string]interface{}{}, // 空路径
		}

		content, _ := json.Marshal(doc)
		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "test-codebase",
			Path:         "test-api.json",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 0, "空路径应该返回 0 个 chunks")
	})

	t.Run("单个路径的文档", func(t *testing.T) {
		doc := map[string]interface{}{
			"openapi": "3.0.0",
			"info": map[string]interface{}{
				"title":   "Test API",
				"version": "1.0.0",
			},
			"paths": map[string]interface{}{
				"/single": map[string]interface{}{
					"get": map[string]interface{}{
						"summary": "Single endpoint",
						"responses": map[string]interface{}{
							"200": map[string]interface{}{
								"description": "Success",
							},
						},
					},
				},
			},
		}

		content, _ := json.Marshal(doc)
		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "test-codebase",
			Path:         "test-api.json",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 1, "单个路径应该返回 1 个 chunk")

		// 验证 chunk 内容
		var chunkDoc map[string]interface{}
		err = json.Unmarshal(chunks[0].Content, &chunkDoc)
		assert.NoError(t, err)

		// 验证标题包含路径信息
		if info, exists := chunkDoc["info"]; exists {
			if infoMap, ok := info.(map[string]interface{}); ok {
				if title, exists := infoMap["title"]; exists {
					titleStr := title.(string)
					assert.Contains(t, titleStr, " - /single", "标题应该包含路径信息")
				}
			}
		}
	})
}

// 测试复杂文档的分割结果
func TestComplexOpenAPIDocumentSplitting(t *testing.T) {
	splitOptions := SplitOptions{
		MaxTokensPerChunk:          1000,
		SlidingWindowOverlapTokens: 100,
		EnableMarkdownParsing:      true,
	}
	splitter, err := NewCodeSplitter(splitOptions)
	assert.NoError(t, err)

	t.Run("Swagger 2.0 JSON 完整文档分割", func(t *testing.T) {
		content, err := os.ReadFile("/home/kcx/codeWorkspace/codebase-embedder/bin/swagger2.json")
		assert.NoError(t, err, "应该能够读取 swagger2.json 文件")

		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "petstore-api",
			Path:         "swagger2.json",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 14, "Swagger 2.0 JSON 应该有 14 个路径")

		// 验证所有路径都被正确分割
		expectedPaths := []string{
			"/pet", "/pet/findByStatus", "/pet/findByTags", "/pet/{petId}",
			"/pet/{petId}/uploadImage", "/store/inventory", "/store/order",
			"/store/order/{orderId}", "/user", "/user/createWithArray",
			"/user/createWithList", "/user/login", "/user/logout", "/user/{username}",
		}

		for i, chunk := range chunks {
			var chunkDoc map[string]interface{}
			err := json.Unmarshal(chunk.Content, &chunkDoc)
			assert.NoError(t, err, "chunk %d 应该是有效的 JSON", i)

			// 验证每个 chunk 只包含一个路径
			if paths, exists := chunkDoc["paths"]; exists {
				if pathsMap, ok := paths.(map[string]interface{}); ok {
					assert.Len(t, pathsMap, 1, "chunk %d 应该只包含一个路径", i)

					// 验证路径名称
					for path := range pathsMap {
						assert.Contains(t, expectedPaths, path, "chunk %d 包含的路径应该在预期列表中", i)
					}
				}
			}

			// 验证保留了所有必要的字段
			assert.Contains(t, chunkDoc, "swagger", "chunk %d 应该包含 swagger 版本", i)
			assert.Contains(t, chunkDoc, "info", "chunk %d 应该包含 info", i)
			assert.Contains(t, chunkDoc, "definitions", "chunk %d 应该包含 definitions", i)
			assert.Contains(t, chunkDoc, "securityDefinitions", "chunk %d 应该包含 securityDefinitions", i)
		}
	})

	t.Run("OpenAPI 3.0 JSON 文档分割", func(t *testing.T) {
		content, err := os.ReadFile("/home/kcx/codeWorkspace/codebase-embedder/bin/openapi3.json")
		assert.NoError(t, err, "应该能够读取 openapi3.json 文件")

		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "petstore-extended-api",
			Path:         "openapi3.json",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 2, "OpenAPI 3.0 JSON 应该有 2 个路径")

		// 验证所有路径都被正确分割
		expectedPaths := []string{"/pets", "/pets/{petId}"}

		for i, chunk := range chunks {
			var chunkDoc map[string]interface{}
			err := json.Unmarshal(chunk.Content, &chunkDoc)
			assert.NoError(t, err, "chunk %d 应该是有效的 JSON", i)

			// 验证每个 chunk 只包含一个路径
			if paths, exists := chunkDoc["paths"]; exists {
				if pathsMap, ok := paths.(map[string]interface{}); ok {
					assert.Len(t, pathsMap, 1, "chunk %d 应该只包含一个路径", i)

					// 验证路径名称
					for path := range pathsMap {
						assert.Contains(t, expectedPaths, path, "chunk %d 包含的路径应该在预期列表中", i)
					}
				}
			}

			// 验证保留了所有必要的字段
			assert.Contains(t, chunkDoc, "openapi", "chunk %d 应该包含 openapi 版本", i)
			assert.Contains(t, chunkDoc, "info", "chunk %d 应该包含 info", i)
			assert.Contains(t, chunkDoc, "components", "chunk %d 应该包含 components", i)
			assert.Contains(t, chunkDoc, "servers", "chunk %d 应该包含 servers", i)
		}
	})

	t.Run("OpenAPI 3.0 YAML 文档分割", func(t *testing.T) {
		content, err := os.ReadFile("/home/kcx/codeWorkspace/codebase-embedder/bin/openapi3.yaml")
		assert.NoError(t, err, "应该能够读取 openapi3.yaml 文件")

		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "user-management-api",
			Path:         "openapi3.yaml",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 2, "OpenAPI 3.0 YAML 应该有 2 个路径")

		// 验证所有路径都被正确分割
		expectedPaths := []string{"/users", "/users/{id}"}

		for i, chunk := range chunks {
			var chunkDoc map[string]interface{}
			err := json.Unmarshal(chunk.Content, &chunkDoc)
			assert.NoError(t, err, "chunk %d 应该是有效的 JSON", i)

			// 验证每个 chunk 只包含一个路径
			if paths, exists := chunkDoc["paths"]; exists {
				if pathsMap, ok := paths.(map[string]interface{}); ok {
					assert.Len(t, pathsMap, 1, "chunk %d 应该只包含一个路径", i)

					// 验证路径名称
					for path := range pathsMap {
						assert.Contains(t, expectedPaths, path, "chunk %d 包含的路径应该在预期列表中", i)
					}
				}
			}

			// 验证保留了所有必要的字段
			assert.Contains(t, chunkDoc, "openapi", "chunk %d 应该包含 openapi 版本", i)
			assert.Contains(t, chunkDoc, "info", "chunk %d 应该包含 info", i)
			assert.Contains(t, chunkDoc, "components", "chunk %d 应该包含 components", i)
			assert.Contains(t, chunkDoc, "servers", "chunk %d 应该包含 servers", i)
		}
	})

	t.Run("Swagger 2.0 YAML 文档分割", func(t *testing.T) {
		content, err := os.ReadFile("/home/kcx/codeWorkspace/codebase-embedder/bin/swagger2.yaml")
		assert.NoError(t, err, "应该能够读取 swagger2.yaml 文件")

		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "user-management-api",
			Path:         "swagger2.yaml",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.IsType(t, err, parser.ErrInvalidOpenAPISpec)
		assert.Len(t, chunks, 0, "Swagger 2.0 YAML 应该有 2 个路径")

		// 验证所有路径都被正确分割
		expectedPaths := []string{"/users", "/users/{id}"}

		for i, chunk := range chunks {
			var chunkDoc map[string]interface{}
			err := json.Unmarshal(chunk.Content, &chunkDoc)
			assert.NoError(t, err, "chunk %d 应该是有效的 JSON", i)

			// 验证每个 chunk 只包含一个路径
			if paths, exists := chunkDoc["paths"]; exists {
				if pathsMap, ok := paths.(map[string]interface{}); ok {
					assert.Len(t, pathsMap, 1, "chunk %d 应该只包含一个路径", i)

					// 验证路径名称
					for path := range pathsMap {
						assert.Contains(t, expectedPaths, path, "chunk %d 包含的路径应该在预期列表中", i)
					}
				}
			}

			// 验证保留了所有必要的字段
			assert.Contains(t, chunkDoc, "swagger", "chunk %d 应该包含 swagger 版本", i)
			assert.Contains(t, chunkDoc, "info", "chunk %d 应该包含 info", i)
			assert.Contains(t, chunkDoc, "definitions", "chunk %d 应该包含 definitions", i)
			assert.Contains(t, chunkDoc, "securityDefinitions", "chunk %d 应该包含 securityDefinitions", i)
		}
	})
}

// 测试错误情况
func TestSplitOpenAPIFileErrorCases(t *testing.T) {
	splitOptions := SplitOptions{
		MaxTokensPerChunk:          1000,
		SlidingWindowOverlapTokens: 100,
		EnableMarkdownParsing:      true,
	}
	splitter, err := NewCodeSplitter(splitOptions)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		content     []byte
		filePath    string
		expectError bool
		description string
	}{
		{
			name:        "无效 JSON",
			content:     []byte(`{ invalid json`),
			filePath:    "test.json",
			expectError: true,
			description: "应该返回 JSON 解析错误",
		},
		{
			name:        "不支持的版本",
			content:     []byte(`{"openapi": "4.0.0", "info": {"title": "test", "version": "1.0.0"}}`),
			filePath:    "test.json",
			expectError: true,
			description: "应该返回不支持的版本错误",
		},
		{
			name:        "缺少必要字段的 OpenAPI 3.0",
			content:     []byte(`{"openapi": "3.0.0", "paths": {}}`),
			filePath:    "test.json",
			expectError: true,
			description: "应该返回验证错误",
		},
		{
			name:        "缺少必要字段的 Swagger 2.0",
			content:     []byte(`{"swagger": "2.0", "info": {"title": "", "version": "1.0.0"}, "paths": {}}`),
			filePath:    "test.json",
			expectError: true,
			description: "应该返回验证错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceFile := &types.SourceFile{
				CodebaseId:   1,
				CodebasePath: "/test/path",
				CodebaseName: "test-codebase",
				Path:         tt.filePath,
				Content:      tt.content,
			}

			chunks, err := splitter.splitOpenAPIFile(sourceFile)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, chunks, "错误时应该返回 nil chunks")
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, chunks, "成功时应该返回非 nil chunks")
			}
		})
	}
}

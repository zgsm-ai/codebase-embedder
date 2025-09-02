package embedding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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

	tests := []struct {
		name        string
		content     []byte
		expectError bool
		expectCount int
		description string
	}{
		{
			name:        "OpenAPI 3.0 Petstore Extended API",
			content:     createOpenAPI3PetstoreExtendedDoc(),
			expectError: false,
			expectCount: 2, // /pets 和 /pets/{petId} 两个路径
			description: "应该成功分割 OpenAPI 3.0 Petstore Extended 文档",
		},
		{
			name:        "Swagger 2.0 Petstore API",
			content:     createSwagger2PetstoreDoc(),
			expectError: false,
			expectCount: 14, // 14个不同的路径
			description: "应该成功分割 Swagger 2.0 Petstore 文档",
		},
		{
			name:        "无效 JSON",
			content:     []byte(`{ invalid json`),
			expectError: true,
			expectCount: 0,
			description: "应该返回 JSON 解析错误",
		},
		{
			name:        "不支持的版本",
			content:     createUnsupportedVersionDoc(),
			expectError: true,
			expectCount: 0,
			description: "应该返回不支持的版本错误",
		},
		{
			name:        "缺少必要字段的 OpenAPI 3.0",
			content:     createInvalidOpenAPI3Doc(),
			expectError: true,
			expectCount: 0,
			description: "应该返回验证错误",
		},
		{
			name:        "缺少必要字段的 Swagger 2.0",
			content:     createInvalidSwagger2Doc(),
			expectError: true,
			expectCount: 0,
			description: "应该返回验证错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试用的 SourceFile
			sourceFile := &types.SourceFile{
				CodebaseId:   1,
				CodebasePath: "/test/path",
				CodebaseName: "test-codebase",
				Path:         "test-api.json",
				Content:      tt.content,
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
		expectError bool
	}{
		{
			name:        "OpenAPI 3.0",
			content:     []byte(`{"openapi": "3.0.3", "info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   OpenAPI3,
			expectError: false,
		},
		{
			name:        "Swagger 2.0",
			content:     []byte(`{"swagger": "2.0", "info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   Swagger2,
			expectError: false,
		},
		{
			name:        "无效 JSON",
			content:     []byte(`{ invalid json`),
			expectVer:   Unknown,
			expectError: true,
		},
		{
			name:        "不支持的版本",
			content:     []byte(`{"openapi": "4.0.0", "info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   Unknown,
			expectError: true,
		},
		{
			name:        "缺少版本字段",
			content:     []byte(`{"info": {"title": "test", "version": "1.0.0"}}`),
			expectVer:   Unknown,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := splitter.validateOpenAPISpec(tt.content)

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

	t.Run("Swagger 2.0 Petstore 完整文档分割", func(t *testing.T) {
		content := createSwagger2PetstoreDoc()
		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "petstore-api",
			Path:         "petstore.json",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 14, "Petstore API 应该有 14 个路径")

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

	t.Run("OpenAPI 3.0 Petstore Extended 文档分割", func(t *testing.T) {
		content := createOpenAPI3PetstoreExtendedDoc()
		sourceFile := &types.SourceFile{
			CodebaseId:   1,
			CodebasePath: "/test/path",
			CodebaseName: "petstore-extended-api",
			Path:         "petstore-extended.json",
			Content:      content,
		}

		chunks, err := splitter.splitOpenAPIFile(sourceFile)
		assert.NoError(t, err)
		assert.Len(t, chunks, 2, "Petstore Extended API 应该有 2 个路径")

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
}

// 辅助函数：创建 OpenAPI 3.0 Petstore Extended 文档
func createOpenAPI3PetstoreExtendedDoc() []byte {
	doc := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "Petstore Extended API",
			"description": "A sample OpenAPI 3.0 JSON file that demonstrates most common constructs.",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name":  "API Support",
				"email": "support@example.com",
			},
			"license": map[string]interface{}{
				"name": "Apache 2.0",
				"url":  "http://www.apache.org/licenses/LICENSE-2.0.html",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "https://api.example.com/v1",
				"description": "Production server",
			},
			{
				"url":         "http://localhost:8080/v1",
				"description": "Local development server",
			},
		},
		"tags": []map[string]interface{}{
			{
				"name":        "pet",
				"description": "Everything about your Pets",
			},
			{
				"name":        "store",
				"description": "Operations about user orders",
			},
		},
		"paths": map[string]interface{}{
			"/pets": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"pet"},
					"summary":     "List all pets",
					"operationId": "listPets",
					"parameters": []map[string]interface{}{
						{
							"$ref": "#/components/parameters/limitParam",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "A list of pets",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/Pet",
										},
									},
								},
							},
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"pet"},
					"summary":     "Create a pet",
					"operationId": "createPet",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/Pet",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Pet created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Pet",
									},
								},
							},
						},
					},
				},
			},
			"/pets/{petId}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"pet"},
					"summary":     "Get pet by ID",
					"operationId": "getPetById",
					"parameters": []map[string]interface{}{
						{
							"name":        "petId",
							"in":          "path",
							"required":    true,
							"description": "ID of pet to fetch",
							"schema": map[string]interface{}{
								"type":   "integer",
								"format": "int64",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Pet details",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Pet",
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Pet": map[string]interface{}{
					"type":     "object",
					"required": []string{"id", "name"},
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":   "integer",
							"format": "int64",
						},
						"name": map[string]interface{}{
							"type": "string",
						},
						"tag": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"Error": map[string]interface{}{
					"type":     "object",
					"required": []string{"code", "message"},
					"properties": map[string]interface{}{
						"code": map[string]interface{}{
							"type":   "integer",
							"format": "int32",
						},
						"message": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
			"parameters": map[string]interface{}{
				"limitParam": map[string]interface{}{
					"name":        "limit",
					"in":          "query",
					"description": "Maximum number of items to return",
					"schema": map[string]interface{}{
						"type":    "integer",
						"format":  "int32",
						"minimum": 1,
						"maximum": 100,
						"default": 20,
					},
				},
			},
			"responses": map[string]interface{}{
				"NotFound": map[string]interface{}{
					"description": "Resource not found",
				},
				"Error": map[string]interface{}{
					"description": "Generic error response",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"$ref": "#/components/schemas/Error",
							},
						},
					},
				},
			},
			"securitySchemes": map[string]interface{}{
				"api_key": map[string]interface{}{
					"type": "apiKey",
					"name": "X-API-KEY",
					"in":   "header",
				},
				"petstore_auth": map[string]interface{}{
					"type": "oauth2",
					"flows": map[string]interface{}{
						"implicit": map[string]interface{}{
							"authorizationUrl": "https://api.example.com/oauth2/authorize",
							"scopes": map[string]interface{}{
								"read:pets":  "Read your pets",
								"write:pets": "Modify your pets",
							},
						},
					},
				},
			},
		},
		"security": []map[string]interface{}{
			{
				"api_key": []interface{}{},
			},
		},
	}

	content, _ := json.Marshal(doc)
	return content
}

// 辅助函数：创建 Swagger 2.0 Petstore 文档
func createSwagger2PetstoreDoc() []byte {
	doc := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":       "Swagger Petstore",
			"description": "This is a sample server Petstore server.",
			"version":     "1.0.3",
			"contact": map[string]interface{}{
				"email": "apiteam@swagger.io",
			},
			"license": map[string]interface{}{
				"name": "Apache 2.0",
				"url":  "http://www.apache.org/licenses/LICENSE-2.0.html",
			},
		},
		"host":     "petstore.swagger.io",
		"basePath": "/v2",
		"schemes":  []string{"https", "http"},
		"paths": map[string]interface{}{
			"/pet": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Add a new pet to the store",
					"tags":        []string{"pet"},
					"operationId": "addPet",
					"responses": map[string]interface{}{
						"405": map[string]interface{}{
							"description": "Invalid input",
						},
					},
				},
				"put": map[string]interface{}{
					"summary":     "Update an existing pet",
					"tags":        []string{"pet"},
					"operationId": "updatePet",
					"responses": map[string]interface{}{
						"400": map[string]interface{}{
							"description": "Invalid ID supplied",
						},
						"404": map[string]interface{}{
							"description": "Pet not found",
						},
					},
				},
			},
			"/pet/findByStatus": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Finds Pets by status",
					"tags":        []string{"pet"},
					"operationId": "findPetsByStatus",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/pet/findByTags": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Finds Pets by tags",
					"tags":        []string{"pet"},
					"operationId": "findPetsByTags",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/pet/{petId}": map[string]interface{}{
				"delete": map[string]interface{}{
					"summary":     "Deletes a pet",
					"tags":        []string{"pet"},
					"operationId": "deletePet",
					"responses": map[string]interface{}{
						"400": map[string]interface{}{
							"description": "Invalid ID supplied",
						},
					},
				},
				"get": map[string]interface{}{
					"summary":     "Find pet by ID",
					"tags":        []string{"pet"},
					"operationId": "getPetById",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/pet/{petId}/uploadImage": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "uploads an image",
					"tags":        []string{"pet"},
					"operationId": "uploadFile",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/store/inventory": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Returns pet inventories by status",
					"tags":        []string{"store"},
					"operationId": "getInventory",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/store/order": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Place an order for a pet",
					"tags":        []string{"store"},
					"operationId": "placeOrder",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/store/order/{orderId}": map[string]interface{}{
				"delete": map[string]interface{}{
					"summary":     "Delete purchase order by ID",
					"tags":        []string{"store"},
					"operationId": "deleteOrder",
					"responses": map[string]interface{}{
						"400": map[string]interface{}{
							"description": "Invalid ID supplied",
						},
					},
				},
				"get": map[string]interface{}{
					"summary":     "Find purchase order by ID",
					"tags":        []string{"store"},
					"operationId": "getOrderById",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/user": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create user",
					"tags":        []string{"user"},
					"operationId": "createUser",
					"responses": map[string]interface{}{
						"default": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/user/createWithArray": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Creates list of users with given input array",
					"tags":        []string{"user"},
					"operationId": "createUsersWithArrayInput",
					"responses": map[string]interface{}{
						"default": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/user/createWithList": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Creates list of users with given input array",
					"tags":        []string{"user"},
					"operationId": "createUsersWithListInput",
					"responses": map[string]interface{}{
						"default": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/user/login": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Logs user into the system",
					"tags":        []string{"user"},
					"operationId": "loginUser",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/user/logout": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Logs out current logged in user session",
					"tags":        []string{"user"},
					"operationId": "logoutUser",
					"responses": map[string]interface{}{
						"default": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
			},
			"/user/{username}": map[string]interface{}{
				"delete": map[string]interface{}{
					"summary":     "Delete user",
					"tags":        []string{"user"},
					"operationId": "deleteUser",
					"responses": map[string]interface{}{
						"400": map[string]interface{}{
							"description": "Invalid username supplied",
						},
					},
				},
				"get": map[string]interface{}{
					"summary":     "Get user by user name",
					"tags":        []string{"user"},
					"operationId": "getUserByName",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "successful operation",
						},
					},
				},
				"put": map[string]interface{}{
					"summary":     "Updated user",
					"tags":        []string{"user"},
					"operationId": "updateUser",
					"responses": map[string]interface{}{
						"400": map[string]interface{}{
							"description": "Invalid user supplied",
						},
					},
				},
			},
		},
		"definitions": map[string]interface{}{
			"Pet": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":   "integer",
						"format": "int64",
					},
					"name": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"User": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":   "integer",
						"format": "int64",
					},
					"username": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		"securityDefinitions": map[string]interface{}{
			"api_key": map[string]interface{}{
				"type": "apiKey",
				"in":   "header",
				"name": "api_key",
			},
			"petstore_auth": map[string]interface{}{
				"type":             "oauth2",
				"flow":             "implicit",
				"authorizationUrl": "https://petstore.swagger.io/oauth/authorize",
				"scopes": map[string]interface{}{
					"read:pets":  "read your pets",
					"write:pets": "modify pets in your account",
				},
			},
		},
		"tags": []map[string]interface{}{
			{
				"name":        "pet",
				"description": "Everything about your Pets",
			},
			{
				"name":        "store",
				"description": "Access to Petstore orders",
			},
			{
				"name":        "user",
				"description": "Operations about user",
			},
		},
	}

	content, _ := json.Marshal(doc)
	return content
}

// 辅助函数：创建不支持的版本文档
func createUnsupportedVersionDoc() []byte {
	doc := map[string]interface{}{
		"openapi": "4.0.0",
		"info": map[string]interface{}{
			"title":   "Test API",
			"version": "1.0.0",
		},
	}

	content, _ := json.Marshal(doc)
	return content
}

// 辅助函数：创建无效的 OpenAPI 3.0 文档
func createInvalidOpenAPI3Doc() []byte {
	doc := map[string]interface{}{
		"openapi": "3.0.0",
		// 缺少 info 字段
		"paths": map[string]interface{}{},
	}

	content, _ := json.Marshal(doc)
	return content
}

// 辅助函数：创建无效的 Swagger 2.0 文档
func createInvalidSwagger2Doc() []byte {
	doc := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":   "", // 空标题
			"version": "1.0.0",
		},
		"paths": map[string]interface{}{},
	}

	content, _ := json.Marshal(doc)
	return content
}

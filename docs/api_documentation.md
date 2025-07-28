# Codebase Embedder API 文档

## 1. 服务简介
Codebase Embedder 提供代码库嵌入管理、语义搜索和项目摘要功能。主要功能包括：
- 提交代码嵌入任务
- 管理嵌入数据
- 获取代码库摘要
- 执行语义代码搜索
- 服务状态检查

## 2. 认证方法
使用 API Key 认证：
- 在请求头中添加 `Authorization` 字段
- 认证方式：`apiKey`
- 示例：
  ```http
  GET /status HTTP/1.1
  Authorization: your_api_key_here
  ```

## 3. 完整端点列表

| 方法   | 端点路径                                  | 功能描述               |
|--------|-------------------------------------------|------------------------|
| POST   | /codebase-embedder/api/v1/embeddings      | 提交嵌入任务           |
| DELETE | /codebase-embedder/api/v1/embeddings      | 删除嵌入数据           |
| GET    | /codebase-embedder/api/v1/embeddings/summary | 获取代码库摘要信息     |
| GET    | /codebase-embedder/api/v1/status          | 服务状态检查           |
| POST   | /codebase-embedder/api/v1/files/status     | 查询文件处理状态       |
| GET    | /codebase-embedder/api/v1/search/semantic | 执行语义代码搜索       |
| POST   | /codebase-embedder/api/v1/files/upload     | 上传文件               |

## 4. 端点详细说明

### 4.1 提交嵌入任务 (POST /embeddings)
**请求示例**：
```json
POST /codebase-embedder/api/v1/embeddings
{
  "clientId": "user_machine_id",
  "projectPath": "/absolute/path/to/project"
}
```

**成功响应**：
```json
HTTP/1.1 200 OK
{
  "taskId": 12345
}
```

### 4.2 删除嵌入数据 (DELETE /embeddings)
**请求示例**：
```http
DELETE /codebase-embedder/api/v1/embeddings
Content-Type: application/x-www-form-urlencoded

clientId=user_machine_id&projectPath=/project/path&filePaths=file1.js,file2.py
```

**成功响应**：
```json
HTTP/1.1 200 OK
{}
```

### 4.3 获取代码库摘要 (GET /embeddings/summary)
**请求示例**：
```http
GET /codebase-embedder/api/v1/embeddings/summary?clientId=user_machine_id&projectPath=/project/path
```

**成功响应**：
```json
HTTP/1.1 200 OK
{
  "embedding": {
    "status": "completed",
    "updatedAt": "2025-07-28T12:00:00Z",
    "totalFiles": 42,
    "totalChunks": 156
  },
  "status": "active",
  "totalFiles": 42
}
```

### 4.4 服务状态检查 (GET /status)
**请求示例**：
```http
GET /codebase-embedder/api/v1/status
```

**成功响应**：
```json
HTTP/1.1 200 OK
{
  "status": "ok",
  "version": "1.0.0"
}
```

### 4.5 语义代码搜索 (GET /search/semantic)
**请求示例**：
```http
GET /codebase-embedder/api/v1/search/semantic?clientId=user_machine_id&projectPath=/project/path&query=authentication+logic&topK=5
```

**成功响应**：
```json
HTTP/1.1 200 OK
{
  "list": [
    {
      "content": "function authenticateUser() {...}",
      "filePath": "src/auth.js",
      "score": 0.92
    },
    {
      "content": "class AuthMiddleware {...}",
      "filePath": "middleware/auth.py",
      "score": 0.87
    }
  ]
}
```

### 4.6 文件状态查询 (POST /files/status)
**请求示例**：
```json
POST /codebase-embedder/api/v1/files/status
{
  "clientId": "user_machine_id",
  "codebasePath": "/absolute/path/to/project",
  "codebaseName": "project_name"
}
```

**成功响应**：
```json
HTTP/1.1 200 OK
{
    "code": 0,
    "message": "ok",
    "data": {
        "process":"processing",// 整体提取状态（如：pending/processing/complete/failed）
        "totalProgress": 50, // 当前分片整体提取进度（百分比，0-100）
        "fileList": [
            {
             "path": "src/main/java/main.java",
             "status": "complete" // 单个文件状态（如：pending/processing/complete/failed）
            },
            {
             "path": "src/main/java/server.java",
             "status": "complete" // 单个文件状态（如：pending/processing/complete/failed）
            }
       ]
    }
}
```

**错误响应**：
```json
HTTP/1.1 404 Not Found
{
  "code": 404,
  "message": "未找到指定的嵌入任务"
}
```

### 4.7 文件上传接口 (POST /files/upload)
**请求格式**：`multipart/form-data`

**参数说明**：
- `clientId`：客户端ID（必填）
- `codebasePath`：项目绝对路径（必填）
- `codebaseName`：项目名称（必填）
- `uploadToken`：上传令牌（必填）
- `extraMetadata`：额外元数据（可选）
- `chunkNumber`：当前分片（可选，默认值0）
- `totalChunks`：分片总数（可选，默认值1）
- `fileTotals`：上传工程文件总数（必填）

**请求示例**：
```http
POST /codebase-embedder/api/v1/files/upload
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="clientId"

user_machine_id
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="codebasePath"

/absolute/path/to/project
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="codebaseName"

project_name
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="fileTotals"

42
------WebKitFormBoundary7MA4YWxkTrZu0gW
```

**成功响应**：
```json
HTTP/1.1 200 OK
{
  "taskId": 12345
}
```

**错误响应**：
```json
HTTP/1.1 400 Bad Request
{
  "code": 400,
  "message": "缺少必需参数: clientId"
}
```

## 5. 标准错误码表

| 错误码 | 含义               | 可能原因                     |
|--------|--------------------|------------------------------|
| 400    | 无效请求参数       | 缺少必需参数/参数格式错误    |
| 401    | 未授权访问         | API Key缺失或无效            |
| 404    | 资源不存在         | 项目/嵌入数据不存在          |
| 500    | 服务器内部错误     | 服务端处理异常               |

**错误响应示例**：
```json
HTTP/1.1 400 Bad Request
{
  "code": 400,
  "message": "缺少必需参数: clientId"
}
# Codebase Embedder 功能文档

## 1. 系统功能概述

Codebase Embedder 是一个代码库嵌入管理系统，旨在为开发人员提供代码库的智能分析和检索能力。系统通过将代码库转换为向量嵌入（embeddings），实现高效的语义搜索和代码理解功能。

核心功能包括：
- **代码库嵌入管理**：将整个代码库上传并转换为向量表示，为后续的语义搜索提供基础
- **语义代码搜索**：基于自然语言查询，查找语义上相关的代码片段，而不仅仅是关键词匹配
- **代码库摘要**：提供代码库的统计信息和处理状态，帮助用户了解代码库的整体情况
- **文件处理状态监控**：实时追踪代码库中各个文件的处理进度和状态
- **服务健康检查**：提供系统状态检查接口，确保服务正常运行

系统采用微服务架构，主要组件包括：
- **HTTP API 服务**：提供RESTful接口供客户端调用
- **向量存储**：使用Weaviate等向量数据库存储代码嵌入
- **任务队列**：管理嵌入任务的执行和调度
- **分布式锁**：确保同一代码库的并发操作安全
- **状态管理**：使用Redis存储临时状态信息

## 2. 各个接口的功能说明

### 2.1 提交嵌入任务 (POST /codebase-embedder/api/v1/embeddings)

**功能描述**：
提交代码库嵌入任务，将本地代码库上传并转换为向量表示。系统会解析代码库中的文件，提取代码结构和语义信息，生成向量嵌入。上传的ZIP文件必须包含`.shenma_sync`文件夹，该文件夹用于存储同步相关的元数据。

**请求参数**（form-data格式）：
- `clientId` (string, 必填): 客户端唯一标识，用于区分不同用户的代码库
- `codebasePath` (string, 必填): 项目在客户端的绝对路径
- `codebaseName` (string, 必填): 代码库名称
- `uploadToken` (string, 可选): 上传令牌，用于验证上传权限（当前调试阶段使用"xxxx"作为万能令牌）
- `fileTotals` (number, 必填): 上传工程文件总数
- `file` (file, 必填): 代码库的ZIP压缩文件，必须包含`.shenma_sync`文件夹
- `extraMetadata` (string, 可选): 额外元数据（JSON字符串格式）
- `X-Request-ID` (header, 可选): 请求ID，用于跟踪和调试，如果没有提供则系统会自动生成

**处理流程**：
1. **参数验证**：验证必填字段（clientId、codebasePath、codebaseName）
2. **令牌验证**：验证uploadToken的有效性（当前调试阶段跳过验证）
3. **代码库初始化**：在数据库中查找或创建代码库记录，使用clientId和codebasePath作为唯一标识
4. **分布式锁获取**：获取基于codebaseID的分布式锁，防止重复处理，锁超时时间可配置
5. **ZIP文件处理**：
   - 验证上传文件为ZIP格式
   - 创建临时文件存储ZIP内容
   - 检查ZIP文件中必须包含`.shenma_sync`文件夹
   - 遍历ZIP文件，读取所有代码文件内容（跳过`.shenma_sync`文件夹中的文件）
   - 读取`.shenma_sync`文件夹中的元数据文件用于任务管理
   - 同步元数据数据格式如下,文件名为时间戳：
   ```json
    {
    "clientId": ""
    "codebasePath": "",
    "codebaseName": "",
    "extraMetadata":  {},
    "fileList":  {
        "src/main/java/main.java": "add" , //add  modify   delete
      },
    "timestamp": 12334234233
    }
    ```

6. **检查文件**：
   - 验证同步元数据文件中add和modify里面文件是否和解压后文件匹配，并打印匹配结果
7. **数据库更新**：更新代码库的文件数量（file_count）和总大小（total_size）信息
8. **任务提交**：将嵌入任务提交到异步任务队列进行处理
9. **状态初始化**：在Redis中使用requestId作为键初始化文件处理状态

**ZIP文件结构要求**：
```
project.zip
├── .shenma_sync/          # 必须存在的文件夹
│   ├── 20250728213645    # 同步元数据文件,文件名为时间戳
│   └── ...               # 其他同步相关文件
├── src/
│   ├── main.js
│   └── utils.js
├── package.json
└── ...                   # 其他项目文件
```

**成功响应**：
```json
{
  "taskId": 12345
}
```

**错误响应**：
- 400 Bad Request：缺少必填参数或ZIP文件格式不正确
- 409 Conflict：无法获取分布式锁，任务正在处理中
- 422 Unprocessable Entity：ZIP文件中缺少必需的`.shenma_sync`文件夹

**注意事项**：
- 上传的ZIP文件大小限制为32MB（可在配置中调整）
- 系统会自动跳过`.shenma_sync`文件夹中的文件，这些文件仅用于任务管理
- 任务处理状态可通过文件状态查询接口进行监控
- 每个代码库（由clientId和codebasePath唯一标识）同时只能有一个处理任务

### 2.2 删除嵌入数据 (DELETE /codebase-embedder/api/v1/embeddings)

**功能描述**：
删除指定代码库的嵌入数据，包括向量存储中的嵌入和数据库中的相关记录。

**请求参数**：
- `clientId` (string): 客户端唯一标识
- `projectPath` (string): 项目路径
- `filePaths` (array, optional): 要删除的特定文件路径列表，为空时删除整个代码库

**成功响应**：
```json
{}
```

### 2.3 获取代码库摘要 (GET /codebase-embedder/api/v1/embeddings/summary)

**功能描述**：
获取指定代码库的摘要信息，包括嵌入状态、文件数量、处理进度等。

**请求参数**：
- `clientId` (string): 客户端唯一标识
- `projectPath` (string): 项目路径

**响应数据**：
```json
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

**字段说明**：
- `embedding.status`: 嵌入处理状态（pending/processing/completed/failed）
- `embedding.updatedAt`: 最后更新时间
- `embedding.totalFiles`: 已处理的文件数量
- `embedding.totalChunks`: 生成的代码块数量
- `totalFiles`: 代码库总文件数量

### 2.4 服务状态检查 (GET /codebase-embedder/api/v1/status)

**功能描述**：
检查服务的健康状态，确认服务是否正常运行。

**成功响应**：
```json
{
  "status": "ok",
  "version": "1.0.0"
}
```

### 2.5 语义代码搜索 (GET /codebase-embedder/api/v1/search/semantic)

**功能描述**：
执行语义代码搜索，根据自然语言查询查找相关的代码片段。系统会将查询转换为向量，并在向量空间中查找最相似的代码块。

**请求参数**：
- `clientId` (string): 客户端唯一标识
- `projectPath` (string): 项目路径
- `query` (string): 搜索查询，可以是自然语言描述
- `topK` (number, optional): 返回结果数量，默认为5
- `scoreThreshold` (number, optional): 相似度分数阈值，过滤低相关性结果

**成功响应**：
```json
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

**字段说明**：
- `content`: 匹配的代码片段内容
- `filePath`: 代码文件路径
- `score`: 相似度分数（0-1），分数越高表示相关性越强

### 2.6 文件状态查询 (POST /codebase-embedder/api/v1/files/status)

**功能描述**：
查询代码库中文件的处理状态，了解嵌入任务的进度。

**请求参数**：
- `clientId` (string): 客户端唯一标识
- `codebasePath` (string): 代码库路径
- `codebaseName` (string): 代码库名称

**成功响应**：
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "process": "processing",
    "totalProgress": 50,
    "fileList": [
      {
        "path": "src/main/java/main.java",
        "status": "complete"
      },
      {
        "path": "src/main/java/server.java",
        "status": "complete"
      }
    ]
  }
}
```

**字段说明**：
- `process`: 整体处理状态（pending/processing/complete/failed）
- `totalProgress`: 整体处理进度百分比（0-100）
- `fileList`: 文件状态列表
  - `path`: 文件路径
  - `status`: 文件处理状态（pending/processing/complete/failed）

### 2.7 文件上传接口 (POST /codebase-embedder/api/v1/files/upload)

**功能描述**：
分片上传代码库文件，适用于大文件上传场景。

**请求参数**：
- `clientId` (string): 客户端唯一标识
- `codebasePath` (string): 项目绝对路径
- `codebaseName` (string): 项目名称
- `uploadToken` (string): 上传令牌
- `extraMetadata` (object, optional): 额外元数据
- `chunkNumber` (number, optional): 当前分片编号，默认为0
- `totalChunks` (number, optional): 分片总数，默认为1
- `fileTotals` (number): 上传工程文件总数
- `file` (file): 当前分片文件

**成功响应**：
```json
{
  "taskId": 12345
}
```

## 3. 使用场景示例

### 3.1 新项目初始化

**场景描述**：
开发人员开始一个新项目，希望快速了解项目结构和关键功能。

**操作步骤**：
1. 使用 `POST /embeddings` 接口提交项目嵌入任务
```bash
curl -X POST "http://localhost:8080/codebase-embedder/api/v1/embeddings" \
  -H "Authorization: your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "clientId": "user123",
    "projectPath": "/home/user/myproject",
    "codebaseName": "myproject",
    "uploadToken": "xxxx",
    "fileTotals": 42
  }' \
  --form "file=@myproject.zip"
```

2. 使用 `GET /embeddings/summary` 接口检查处理进度
```bash
curl -X GET "http://localhost:8080/codebase-embedder/api/v1/embeddings/summary?clientId=user123&projectPath=/home/user/myproject" \
  -H "Authorization: your_api_key"
```

3. 项目处理完成后，使用 `GET /search/semantic` 进行语义搜索
```bash
curl -X GET "http://localhost:8080/codebase-embedder/api/v1/search/semantic?clientId=user123&projectPath=/home/user/myproject&query=authentication%20logic&topK=5" \
  -H "Authorization: your_api_key"
```

### 3.2 代码库迁移

**场景描述**：
团队将旧项目迁移到新服务器，需要验证迁移后的代码库是否完整。

**操作步骤**：
1. 提交新代码库的嵌入任务
2. 使用 `POST /files/status` 接口监控处理进度
3. 比较新旧代码库的摘要信息，确保文件数量和结构一致
4. 执行相同的语义搜索查询，验证搜索结果的一致性

### 3.3 代码审查辅助

**场景描述**：
进行代码审查时，需要快速了解相关代码的上下文。

**操作步骤**：
1. 使用语义搜索查找与审查功能相关的代码
2. 根据搜索结果中的文件路径，快速定位相关文件
3. 查看搜索结果中的代码片段，了解功能实现的上下文
4. 使用不同的查询词进行多轮搜索，全面了解代码库

## 4. 用户操作流程

### 4.1 初始设置

1. **获取API密钥**：联系系统管理员获取访问API的密钥
2. **准备代码库**：将要分析的代码库压缩为ZIP文件
3. **确定客户端ID**：为当前设备或用户分配唯一的客户端标识

### 4.2 提交嵌入任务

1. **调用嵌入接口**：使用 `POST /embeddings` 提交代码库
   - 确保提供正确的 `clientId`、`projectPath` 和 `codebaseName`
   - 上传完整的代码库ZIP文件
   - 提供正确的 `uploadToken`

2. **监控处理进度**：
   - 使用 `GET /status` 确认服务正常
   - 使用 `GET /embeddings/summary` 或 `POST /files/status` 查询处理进度
   - 处理状态会经历 `pending` → `processing` → `completed` 的变化

3. **处理完成**：
   - 当状态变为 `completed` 时，嵌入任务完成
   - 可以开始进行语义搜索和其他操作

### 4.3 日常使用

1. **语义搜索**：
   - 使用自然语言描述要查找的功能
   - 调整 `topK` 参数控制返回结果数量
   - 根据 `score` 字段评估结果的相关性

2. **状态管理**：
   - 定期检查代码库状态，确保数据最新
   - 如果代码库有重大更新，重新提交嵌入任务

3. **数据清理**：
   - 使用 `DELETE /embeddings` 删除不再需要的代码库数据
   - 可以选择删除整个代码库或特定文件

### 4.4 故障排除

**常见问题及解决方案**：

1. **提交任务失败**：
   - 检查API密钥是否正确
   - 确认 `uploadToken` 有效
   - 验证代码库ZIP文件格式正确

2. **处理进度停滞**：
   - 检查服务日志，确认没有错误
   - 使用 `GET /status` 确认服务正常运行
   - 重启嵌入任务

3. **搜索结果不相关**：
   - 尝试不同的查询词
   - 降低 `scoreThreshold` 以获得更多信息
   - 重新提交嵌入任务，确保代码库已完全处理

通过遵循上述操作流程，用户可以充分利用Codebase Embedder的功能，提高代码理解和开发效率。
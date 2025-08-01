# 技术设计文档 - 代码库向量数据库查询接口

## 1. 设计概述

### 1.1 设计目标
基于需求规格说明书，设计一个高性能、安全可靠的代码库向量数据库查询接口，支持通过clientId、codebasePath、codebaseName查询工程向量数据库的完整信息。

### 1.2 设计范围
本技术设计涵盖系统架构集成、API接口设计、Weaviate查询优化、模块分层设计、错误处理、性能优化、安全认证和测试策略等关键技术方案。

## 2. 系统架构设计

### 2.1 整体架构图

```mermaid
graph TB
    subgraph "API网关层"
        A[HTTP请求] --> B[Token认证中间件]
        B --> C[查询接口Handler]
    end
    
    subgraph "业务逻辑层"
        C --> D[权限验证Logic]
        D --> E[数据查询Logic]
        E --> F[数据聚合Logic]
    end
    
    subgraph "数据存储层"
        F --> G[PostgreSQL Store]
        F --> H[Weaviate Store]
    end
    
    subgraph "外部依赖"
        G --> I[codebase表]
        H --> J[向量数据库]
    end
    
    C -.-> K[错误处理中间件]
    K -.-> L[统一响应格式]
```

### 2.2 架构集成设计
- **现有架构兼容性**: 完全兼容现有的internal/handler、internal/logic、internal/store三层架构
- **模块扩展性**: 新增查询模块独立部署，不影响现有功能
- **数据一致性**: 通过事务保证PostgreSQL和Weaviate数据的一致性

## 3. API接口设计

### 3.1 RESTful接口规范

#### 3.1.1 接口详情
- **端点**: `GET /codebase-embedder/api/v1/codebase/query`
- **描述**: 根据clientId、codebasePath、codebaseName查询代码库向量数据库
- **版本**: v1
- **认证**: Bearer Token

#### 3.1.2 请求参数
| 参数名称 | 类型 | 位置 | 必填 | 描述 | 验证规则 |
|----------|------|------|------|------|----------|
| clientId | string | query | 是 | 客户端标识 | `^[a-zA-Z0-9_-]{1,64}$` |
| codebasePath | string | query | 是 | 代码库路径 | 最大长度512字符 |
| codebaseName | string | query | 是 | 代码库名称 | `^[a-zA-Z0-9_-]{1,128}$` |

#### 3.1.3 请求示例
```bash
GET /codebase-embedder/api/v1/codebase/query?clientId=dev-001&codebasePath=/home/user/project&codebaseName=myapp
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

### 3.2 响应数据结构

#### 3.2.1 成功响应 (200 OK)
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "codebaseId": 12345,
    "codebaseName": "myapp",
    "codebasePath": "/home/user/project",
    "summary": {
      "totalFiles": 150,
      "totalChunks": 3200,
      "lastUpdateTime": "2025-07-29T10:30:00Z",
      "indexStatus": "completed",
      "indexProgress": 100
    },
    "languageDistribution": [
      {
        "language": "go",
        "fileCount": 80,
        "chunkCount": 1800,
        "percentage": 56.25
      },
      {
        "language": "javascript",
        "fileCount": 45,
        "chunkCount": 950,
        "percentage": 29.69
      }
    ],
    "recentFiles": [
      {
        "filePath": "src/main.go",
        "lastIndexed": "2025-07-29T10:25:00Z",
        "chunkCount": 25,
        "fileSize": 2048
      }
    ],
    "indexStats": {
      "averageChunkSize": 150,
      "maxChunkSize": 512,
      "minChunkSize": 10,
      "totalVectors": 3200
    }
  }
}
```

#### 3.2.2 错误响应
```json
{
  "code": 403,
  "message": "Permission denied: client does not have access to this codebase",
  "data": null
}
```

## 4. 数据库查询设计

### 4.1 Weaviate查询设计

#### 4.1.1 GraphQL查询结构
```graphql
{
  Get {
    CodeChunk(
      where: {
        operator: And
        operands: [
          { path: ["codebaseId"], operator: Equal, valueInt: 12345 }
        ]
      }
    ) {
      filePath
      language
      chunkIndex
      content
      tokenCount
      lastIndexed
    }
  }
}

{
  Aggregate {
    CodeChunk(
      where: {
        operator: And
        operands: [
          { path: ["codebaseId"], operator: Equal, valueInt: 12345 }
        ]
      }
    ) {
      meta {
        count
      }
      filePath {
        count
        topOccurrences {
          value
          occurs
        }
      }
      language {
        count
        topOccurrences {
          value
          occurs
        }
      }
    }
  }
}
```

#### 4.1.2 索引优化策略
- **向量索引**: HNSW算法，M=16, efConstruction=128
- **标量索引**: 为codebaseId、language、filePath建立倒排索引
- **复合索引**: (codebaseId, language)联合索引加速分组查询

### 4.2 PostgreSQL查询设计

#### 4.2.1 权限验证查询
```sql
-- 验证clientId与codebase的关联
SELECT id, name, client_path, file_count, total_size, status
FROM codebase 
WHERE client_id = ? AND name = ? AND client_path = ?
LIMIT 1;
```

#### 4.2.2 性能优化
- **索引设计**: 
  - 主键索引: `PRIMARY KEY (id)`
  - 复合索引: `INDEX idx_client_codebase (client_id, name, client_path)`
  - 覆盖索引: `INDEX idx_status (status, updated_at)`

## 5. 模块设计

### 5.1 分层架构设计

#### 5.1.1 Handler层 (internal/handler/codebase_query.go)
```go
type CodebaseQueryHandler struct {
    svcCtx *svc.ServiceContext
}

func (h *CodebaseQueryHandler) QueryCodebase(w http.ResponseWriter, r *http.Request) {
    // 1. 参数验证
    // 2. 权限检查
    // 3. 调用Logic层
    // 4. 返回响应
}
```

#### 5.1.2 Logic层 (internal/logic/codebase_query_logic.go)
```go
type CodebaseQueryLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func (l *CodebaseQueryLogic) QueryCodebase(req *types.CodebaseQueryRequest) (*types.CodebaseQueryResponse, error) {
    // 1. 权限验证
    // 2. 获取codebaseId
    // 3. 查询Weaviate统计数据
    // 4. 聚合和格式化数据
    // 5. 返回结果
}
```

#### 5.1.3 Store层
- **PostgreSQL Store** (internal/store/database/codebase_store.go)
- **Weaviate Store** (internal/store/vector/codebase_vector_store.go)

### 5.2 核心数据结构

#### 5.2.1 请求结构
```go
type CodebaseQueryRequest struct {
    ClientId     string `json:"clientId" validate:"required,len=1,64"`
    CodebasePath string `json:"codebasePath" validate:"required,len=1,512"`
    CodebaseName string `json:"codebaseName" validate:"required,len=1,128"`
}
```

#### 5.2.2 响应结构
```go
type CodebaseQueryResponse struct {
    CodebaseId   int32                    `json:"codebaseId"`
    CodebaseName string                   `json:"codebaseName"`
    CodebasePath string                   `json:"codebasePath"`
    Summary      *CodebaseSummary         `json:"summary"`
    LanguageDist []LanguageDistribution   `json:"languageDistribution"`
    RecentFiles  []RecentFileInfo         `json:"recentFiles"`
    IndexStats   *IndexStatistics         `json:"indexStats"`
}
```

## 6. 错误处理机制

### 6.1 错误分类与处理

| 错误类型 | HTTP状态码 | 错误消息 | 处理策略 |
|----------|------------|----------|----------|
| 参数验证错误 | 400 | Invalid parameters | 详细字段级错误信息 |
| 权限拒绝 | 403 | Permission denied | 不暴露内部细节 |
| 数据不存在 | 404 | Codebase not found | 友好提示 |
| 数据库错误 | 500 | Internal server error | 记录日志，返回通用错误 |
| 查询超时 | 504 | Query timeout | 异步处理或重试机制 |

### 6.2 错误处理流程
```mermaid
graph TD
    A[请求处理] --> B{发生错误?}
    B -->|是| C[错误分类]
    C --> D[记录日志]
    D --> E[返回标准化错误响应]
    B -->|否| F[返回成功响应]
```

## 7. 性能优化策略

### 7.1 查询优化
- **预聚合查询**: 使用Weaviate的Aggregate API减少数据传输
- **分页查询**: 对recentFiles实施分页，默认返回最近10个文件
- **字段选择**: 只查询必要字段，减少网络传输
- **连接池**: 复用数据库连接，减少连接开销

### 7.2 缓存策略
- **无缓存设计**: 为保证数据实时性，不引入缓存
- **查询优化**: 通过数据库索引和查询优化提升性能

### 7.3 并发控制
- **限流机制**: 基于clientId的令牌桶限流，100 QPS/client
- **连接池配置**: 
  - PostgreSQL: 最大连接数50
  - Weaviate: 最大并发连接数20

### 7.4 性能指标
| 指标 | 目标值 | 监控方法 |
|------|--------|----------|
| P95响应时间 | ≤500ms | Prometheus监控 |
| 并发处理能力 | ≥100并发 | 负载测试 |
| 错误率 | <0.1% | 错误日志监控 |
| 数据库查询时间 | ≤100ms | 慢查询日志 |

## 8. 安全认证机制

### 8.1 认证流程
```mermaid
sequenceDiagram
    Client->>API: 请求 + Token
    API->>Auth: 验证Token
    Auth->>API: 返回clientId
    API->>DB: 验证clientId权限
    DB->>API: 返回验证结果
    API->>Client: 返回查询结果或错误
```

### 8.2 权限验证
- **Token验证**: 使用JWT Token验证用户身份
- **权限检查**: 验证clientId与codebasePath的关联关系
- **审计日志**: 记录所有查询操作
  - 用户标识
  - 查询参数
  - 查询时间
  - 响应状态

### 8.3 安全防护措施
- **输入验证**: 严格验证所有输入参数
- **SQL注入防护**: 使用参数化查询
- **XSS防护**: 对所有输出进行HTML转义
- **速率限制**: 基于IP和clientId的双重限流

## 9. 测试策略

### 9.1 单元测试
- **测试覆盖率**: ≥80%
- **测试内容**:
  - 参数验证逻辑
  - 权限验证逻辑
  - 数据聚合逻辑
  - 错误处理逻辑

### 9.2 集成测试
- **测试场景**:
  - 正常查询流程
  - 权限验证失败
  - 数据不存在
  - 数据库连接失败
  - 并发查询测试

### 9.3 性能测试
- **测试工具**: k6
- **测试场景**:
  - 100并发用户持续5分钟
  - 大数据集查询测试
  - 内存和CPU使用率监控

### 9.4 安全测试
- **测试内容**:
  - SQL注入测试
  - XSS攻击测试
  - 权限绕过测试
  - 速率限制测试

### 9.5 测试数据准备
```go
// 测试数据生成器
func generateTestCodebase() *model.Codebase {
    return &model.Codebase{
        ID:         12345,
        ClientID:   "test-client-001",
        UserID:     "user@example.com",
        Name:       "test-project",
        ClientPath: "/home/test/project",
        Status:     "completed",
        FileCount:  150,
        TotalSize:  1024000,
    }
}
```

## 10. 部署与监控

### 10.1 部署策略
- **蓝绿部署**: 零停机时间部署
- **健康检查**: `/health`端点监控服务状态
- **配置管理**: 使用环境变量管理配置

### 10.2 监控指标
- **业务指标**:
  - 查询成功率
  - 平均响应时间
  - 错误率分布
- **系统指标**:
  - CPU使用率
  - 内存使用率
  - 数据库连接数
  - 响应时间分布

### 10.3 告警机制
- **响应时间告警**: P95 > 500ms
- **错误率告警**: 错误率 > 1%
- **数据库连接告警**: 连接数 > 80%

## 11. 技术选型总结

| 组件类型 | 技术选型 | 版本 | 选择理由 |
|----------|----------|------|----------|
| Web框架 | Go标准库 net/http | 1.19+ | 轻量级，性能优秀 |
| 数据库 | PostgreSQL | 13+ | 成熟稳定，支持复杂查询 |
| 向量数据库 | Weaviate | 1.18+ | 专为向量搜索优化 |
| 验证库 | go-playground/validator | v10 | 功能强大，社区活跃 |
| 监控 | Prometheus + Grafana | 最新版 | 云原生标准监控方案 |
| 限流 | golang.org/x/time/rate | 标准库 | 官方支持，稳定可靠 |

## 12. 提交嵌入任务技术设计

### 12.1 任务提交流程

#### 12.1.1 整体流程图
```mermaid
sequenceDiagram
    participant Client
    participant API
    participant Storage
    participant Queue
    participant Worker
    
    Client->>API: POST /embeddings (multipart/form-data)
    API->>API: 验证参数和Token
    API->>Storage: 查找/创建codebase记录
    API->>Storage: 获取分布式锁
    API->>Storage: 保存ZIP文件到临时目录
    API->>Storage: 解压并验证ZIP结构
    API->>Storage: 更新codebase统计信息
    API->>Queue: 提交异步嵌入任务
    API->>Storage: 初始化Redis任务状态
    API->>Client: 返回taskId
    Queue->>Worker: 触发任务执行
    Worker->>Worker: 执行嵌入和图构建
    Worker->>Storage: 更新任务状态
```

#### 12.1.2 关键处理步骤

**1. 请求验证阶段**
- 必填参数验证：`clientId`, `codebasePath`, `codebaseName`, `fileTotals`
- 文件验证：必须是ZIP格式，且必须包含`.shenma_sync/`文件夹
- Token验证：当前调试阶段使用万能令牌"xxxx"

**2. 文件处理阶段**
- 临时文件存储：上传的ZIP文件保存到系统临时目录
- 文件结构验证：检查ZIP文件中必须存在`.shenma_sync/`文件夹
- 文件内容读取：分别读取普通代码文件和`.shenma_sync/`中的元数据文件
- 统计信息更新：更新codebase的file_count和total_size字段

**3. 任务调度阶段**
- 分布式锁：使用Redis分布式锁防止重复处理，锁超时时间可配置
- 异步任务：通过任务池提交异步嵌入任务
- 状态管理：使用RequestId作为键在Redis中存储任务状态

### 12.2 接口设计

#### 12.2.1 请求规范
- **端点**: `POST /codebase-embedder/api/v1/embeddings`
- **内容类型**: `multipart/form-data`
- **最大文件大小**: 32MB

#### 12.2.2 请求参数
| 参数名称 | 类型 | 位置 | 必填 | 描述 | 验证规则 |
|----------|------|------|------|------|----------|
| clientId | string | form-data | 是 | 客户端唯一标识 | 非空字符串 |
| codebasePath | string | form-data | 是 | 代码库绝对路径 | 非空字符串 |
| codebaseName | string | form-data | 是 | 代码库名称 | 非空字符串 |
| uploadToken | string | form-data | 否 | 上传令牌 | 当前调试阶段为"xxxx" |
| fileTotals | int | form-data | 是 | 文件总数 | 正整数 |
| file | file | form-data | 是 | ZIP压缩文件 | 必须为.zip格式 |
| extraMetadata | string | form-data | 否 | 额外元数据 | JSON格式字符串 |
| X-Request-ID | string | header | 否 | 请求ID | 用于状态跟踪 |

#### 12.2.3 ZIP文件结构要求
```
project.zip
├── .shenma_sync/          # 必须存在
│   ├── config.json        # 同步配置
│   └── metadata.json      # 项目元数据
├── src/                   # 源代码目录
├── README.md              # 项目文档
└── ...                    # 其他项目文件
```

### 12.3 任务处理机制

#### 12.3.1 分布式锁设计
- **锁键格式**: `codebase_embedder:task:{codebaseId}`
- **锁超时**: 可配置，默认5分钟
- **锁实现**: 基于Redis的Redsync库

#### 12.3.2 异步任务结构
```go
type IndexTask struct {
    SvcCtx  *svc.ServiceContext
    LockMux *redsync.Mutex
    Params  *IndexTaskParams
}

type IndexTaskParams struct {
    CodebaseID   int32              // 代码库ID
    CodebasePath string            // 代码库路径
    CodebaseName string            // 代码库名称
    ClientId     string            // 客户端ID
    RequestId    string            // 请求ID
    Files        map[string][]byte // 文件内容映射
}
```

#### 12.3.3 任务执行流程
1. **解锁保证**: 任务完成后自动释放分布式锁
2. **并发处理**: 使用WaitGroup管理嵌入任务和图构建任务
3. **超时控制**: 任务超时时间可配置
4. **错误处理**: 完整的错误捕获和日志记录

### 12.4 状态管理

#### 12.4.1 Redis状态存储
- **键格式**: 使用RequestId作为状态键
- **初始状态**: `{"process": "pending", "totalProgress": 0, "fileList": []}`
- **状态更新**: 任务执行过程中实时更新处理进度

#### 12.4.2 状态查询接口
- **端点**: `GET /codebase-embedder/api/v1/status/{requestId}`
- **轮询机制**: 客户端可通过轮询获取实时处理状态

### 12.5 错误处理机制

#### 12.5.1 错误分类
| 错误类型 | HTTP状态码 | 错误场景 | 处理策略 |
|----------|------------|----------|----------|
| 参数错误 | 400 | 缺少必填参数 | 返回具体缺失参数信息 |
| 文件格式错误 | 400 | ZIP格式不正确或缺少.shenma_sync | 返回具体格式要求 |
| 权限错误 | 403 | Token验证失败 | 返回权限不足信息 |
| 锁冲突 | 409 | 分布式锁获取失败 | 返回任务正在处理中 |
| 服务器错误 | 500 | 文件处理或数据库错误 | 记录日志，返回通用错误 |

#### 12.5.2 错误响应格式
```json
{
  "code": 400,
  "message": "Invalid parameters: missing required field 'clientId'",
  "data": null
}
```

### 12.6 性能优化

#### 12.6.1 文件处理优化
- **内存管理**: 使用临时文件避免内存溢出
- **并发读取**: ZIP文件并发读取多个文件
- **垃圾回收**: 及时清理临时文件和释放内存

#### 12.6.2 任务调度优化
- **连接池**: 复用数据库连接
- **批处理**: 批量处理文件减少数据库操作
- **异步化**: 非阻塞式任务提交

### 12.7 安全设计

#### 12.7.1 文件安全
- **文件类型检查**: 严格限制上传文件类型为ZIP
- **文件大小限制**: 32MB最大限制防止DoS攻击
- **路径遍历防护**: 防止ZIP文件中的路径遍历攻击

#### 12.7.2 数据安全
- **临时文件清理**: 自动清理上传的临时文件
- **敏感信息过滤**: 过滤代码中的敏感信息
- **访问控制**: 基于clientId的权限验证

### 12.8 监控与日志

#### 12.8.1 关键指标监控
- **任务提交成功率**: 成功提交任务的比例
- **平均处理时间**: 从提交到完成的平均时间
- **错误率**: 各类错误的分布情况
- **文件处理统计**: 处理的文件数量和大小分布

#### 12.8.2 日志记录
- **请求日志**: 记录所有任务提交请求
- **处理日志**: 记录任务执行的详细步骤
- **错误日志**: 记录所有错误和异常信息
- **性能日志**: 记录关键操作的执行时间

## 13. 后续扩展计划

### 13.1 功能扩展
- 支持更多代码库格式（Git仓库直接导入）
- 支持增量更新（只处理变更的文件）
- 支持代码库版本管理

### 13.2 性能优化
- 支持大文件分片上传
- 实现断点续传功能
- 引入CDN加速文件上传

### 13.3 安全增强
- 实现OAuth2.0认证
- 支持API访问频率限制
- 实现代码库访问审计日志
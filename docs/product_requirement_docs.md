# 需求规格说明书 - 代码库向量数据库查询接口

## 1. 项目概述

### 1.1 背景
Codebase Embedder系统需要新增一个查询接口，用于根据clientId、codebasePath、codebaseName读取工程向量数据库的详细信息。该接口将为开发者提供代码库索引状态的完整概览，包括统计信息、文件分布和最近更新情况。

### 1.2 目标
- 提供基于clientId、codebasePath、codebaseName的代码库向量数据查询能力
- 返回完整的代码库索引状态和统计信息
- 确保严格的权限验证，防止越权访问
- 提供实时准确的数据，不引入缓存延迟
- 支持MVP版本快速上线，后续可扩展更多查询维度

### 1.3 范围
本文档定义了代码库向量数据库查询接口的完整需求，包括功能需求、接口规范、数据格式、安全要求和性能标准。该接口将作为现有RESTful API体系的补充，专门用于代码库级别的向量数据查询。

## 2. 功能需求

### 2.1 用户角色
| 角色名称 | 描述 | 权限 |
|----------|------|------|
| 开发者 | 使用代码库查询功能的终端用户 | 查询自己拥有的代码库 |
| 系统管理员 | 监控系统运行状态的管理员 | 查询所有代码库状态 |

### 2.2 功能清单

#### 2.2.1 代码库向量数据查询
- **需求ID**: FR-CDQ-001
- **EARS描述**:
  - **E**vent: 当用户提交包含clientId、codebasePath、codebaseName的有效查询请求时
  - **A**ction: 系统应该验证用户权限并返回该代码库的完整向量数据概览
  - **R**esult: 返回包含统计信息、文件分布、最近更新等详细数据
  - **S**takeholder: 需要了解代码库索引状态的开发者

- **需求描述**: 提供基于三个核心参数的代码库向量数据查询接口，确保数据的完整性和实时性

- **优先级**: 高
- **验收标准**: 
  - 响应时间 ≤ 500ms
  - 数据准确率 = 100%
  - 权限验证通过率 = 100%

#### 2.2.2 权限验证机制
- **需求ID**: FR-CDQ-002
- **EARS描述**:
  - **E**vent: 当用户查询代码库向量数据时
  - **A**ction: 系统应该验证clientId与codebasePath的关联关系
  - **R**esult: 拒绝无权限访问并返回403错误
  - **S**takeholder: 系统安全管理员

- **需求描述**: 建立严格的权限验证机制，确保用户只能访问自己拥有的代码库数据

#### 2.2.3 实时数据保证
- **需求ID**: FR-CDQ-003
- **EARS描述**:
  - **E**vent: 当用户查询代码库状态时
  - **A**ction: 系统应该直接从向量数据库获取最新数据
  - **R**esult: 返回无缓存延迟的实时准确数据
  - **S**takeholder: 需要准确索引状态的开发者

## 3. 接口规范

### 3.1 API端点
- **端点**: GET /codebase-embedder/api/v1/codebase/query
- **功能**: 根据clientId、codebasePath、codebaseName查询代码库向量数据库
- **版本**: v1
- **协议**: RESTful

### 3.2 输入参数规范

#### 3.2.1 查询参数
| 参数名称 | 类型 | 必填 | 描述 | 验证规则 |
|----------|------|------|------|----------|
| clientId | string | 是 | 用户机器唯一标识 | 长度1-64字符，只允许字母、数字、下划线 |
| codebasePath | string | 是 | 项目绝对路径 | 长度1-512字符，必须为有效路径格式 |
| codebaseName | string | 是 | 代码库名称 | 长度1-128字符，不允许特殊字符 |

#### 3.2.2 参数验证规则
- **clientId**: 必须匹配已注册的用户标识
- **codebasePath**: 必须符合操作系统路径规范
- **codebaseName**: 必须与实际代码库名称一致

### 3.3 输出数据结构

#### 3.3.1 成功响应格式
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "codebaseId": 12345,
    "codebaseName": "example-project",
    "codebasePath": "/home/user/projects/example",
    "summary": {
      "totalFiles": 150,
      "totalChunks": 3200,
      "lastUpdateTime": "2025-07-29T10:30:00Z",
      "indexStatus": "completed"
    },
    "languageDistribution": [
      {
        "language": "go",
        "fileCount": 80,
        "chunkCount": 1800
      },
      {
        "language": "javascript",
        "fileCount": 45,
        "chunkCount": 950
      },
      {
        "language": "python",
        "fileCount": 25,
        "chunkCount": 450
      }
    ],
    "recentFiles": [
      {
        "filePath": "src/main.go",
        "lastIndexed": "2025-07-29T10:25:00Z",
        "chunkCount": 25
      },
      {
        "filePath": "src/utils/helper.js",
        "lastIndexed": "2025-07-29T10:20:00Z",
        "chunkCount": 15
      }
    ],
    "indexStats": {
      "averageChunkSize": 150,
      "maxChunkSize": 512,
      "minChunkSize": 10
    }
  }
}
```

#### 3.3.2 错误响应格式
```json
{
  "code": 403,
  "message": "Permission denied: client does not have access to this codebase",
  "data": null
}
```

## 4. 非功能需求

### 4.1 性能需求
- **响应时间**: 95%的请求响应时间 ≤ 500ms
- **并发能力**: 支持100个并发查询
- **吞吐量**: ≥ 200 QPS
- **可用性**: 99.9%可用性保证

### 4.2 安全需求
- **认证要求**: 必须验证clientId有效性
- **授权要求**: 必须验证clientId与codebasePath的关联关系
- **数据保护**: 不返回敏感文件内容，仅返回统计信息
- **审计日志**: 记录所有查询操作，包括时间、用户、查询参数

### 4.3 兼容性需求
- **路径格式**: 支持Windows、Linux、macOS路径格式
- **编码支持**: 支持UTF-8、GBK等常见编码
- **API版本**: 向后兼容，支持平滑升级

## 5. 用户故事

### 5.1 开发者查询代码库状态
**作为** 一个开发者
**我想要** 通过clientId、codebasePath、codebaseName查询我的代码库索引状态
**以便于** 了解代码向量化的完整情况和最近更新

**验收条件**:
- 只能查询自己拥有的代码库
- 返回完整的状态概览
- 响应时间在可接受范围内

### 5.2 系统管理员监控代码库
**作为** 系统管理员
**我想要** 查询任意代码库的向量数据状态
**以便于** 监控系统整体运行状况

**验收条件**:
- 具有超级管理员权限
- 可以查询所有代码库状态
- 提供批量查询能力

## 6. 数据需求

### 6.1 数据实体
- **代码库信息**: codebaseId, codebaseName, codebasePath, clientId
- **索引统计**: totalFiles, totalChunks, lastUpdateTime, indexStatus
- **语言分布**: language, fileCount, chunkCount
- **文件信息**: filePath, lastIndexed, chunkCount

### 6.2 数据流
1. 接收查询请求并验证参数
2. 验证clientId与codebasePath的关联关系
3. 通过codebasePath和codebaseName查询codebaseId
4. 从Weaviate向量数据库获取统计数据
5. 聚合和格式化返回数据

## 7. 接口需求

### 7.1 外部接口
- **Weaviate向量数据库**: 获取代码库索引数据
- **PostgreSQL数据库**: 验证codebaseId和权限关系

### 7.2 内部接口
- **权限验证服务**: 验证clientId与codebase的关联
- **数据聚合服务**: 聚合和格式化查询结果

## 8. 约束条件

### 8.1 技术约束
- 必须使用现有的Weaviate客户端接口
- 必须兼容现有的数据库模型
- 必须遵循现有的错误处理规范

### 8.2 业务约束
- 不能暴露敏感代码内容
- 必须保证数据实时性
- 必须支持现有用户体系

### 8.3 法规约束
- 符合数据隐私保护要求
- 符合软件出口管制规定

## 9. 假设和依赖

### 9.1 假设
- Weaviate向量数据库正常运行
- PostgreSQL数据库包含有效的codebaseId映射
- clientId与codebasePath的关联关系已建立

### 9.2 依赖
- 依赖现有的Weaviate包装器实现
- 依赖现有的数据库查询层
- 依赖现有的权限验证机制

## 10. 风险分析

| 风险描述 | 概率 | 影响 | 缓解策略 |
|----------|------|------|----------|
| Weaviate查询性能瓶颈 | 中 | 高 | 实施查询优化和索引策略 |
| 权限验证逻辑复杂 | 低 | 中 | 复用现有权限验证机制 |
| 大数据集查询超时 | 中 | 中 | 实施分页和超时机制 |
| 路径格式兼容性问题 | 低 | 低 | 统一路径标准化处理 |

## 11. 验收标准

### 11.1 功能验收标准
| 测试维度 | 验收标准 | 测量方法 | 通过阈值 |
|----------|----------|----------|----------|
| 查询准确性 | 返回数据与数据库一致 | 数据对比测试 | 100%准确 |
| 权限验证 | 正确拦截无权限访问 | 权限测试用例 | 100%拦截 |
| 数据完整性 | 返回所有必要字段 | 字段完整性检查 | 100%覆盖 |
| 错误处理 | 清晰的错误信息 | 错误场景测试 | 信息可理解 |

### 11.2 性能验收标准
| 性能指标 | 目标值 | 测试条件 | 测量方法 |
|----------|--------|----------|----------|
| 查询响应时间 | ≤500ms | 100并发查询 | P95响应时间 |
| 并发处理能力 | ≥100并发 | 持续5分钟 | 成功率≥99% |
| 系统资源使用 | CPU≤70%, 内存≤2GB | 峰值负载 | 系统监控 |

### 11.3 安全验收标准
| 安全要求 | 验收标准 | 测试方法 | 通过条件 |
|----------|----------|----------|----------|
| 权限控制 | 防止越权访问 | 横向权限测试 | 100%拦截 |
| 输入验证 | 拒绝恶意输入 | 注入攻击测试 | 0个安全漏洞 |
| 数据保护 | 不泄露敏感信息 | 数据泄露测试 | 无敏感信息暴露 |

## 12. 附录

### 12.1 术语表
- **EARS**: Easy Approach to Requirements Syntax，一种需求描述语法
- **MVP**: Minimum Viable Product，最小可行产品
- **QPS**: Queries Per Second，每秒查询数
- **P95**: 95百分位响应时间

### 12.2 参考资料
- [Weaviate GraphQL API文档](https://weaviate.io/developers/weaviate/api/graphql)
- [RESTful API设计最佳实践](https://restfulapi.net/)
- [Go语言HTTP服务开发指南](https://golang.org/doc/articles/wiki/)
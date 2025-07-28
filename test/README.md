# Codebase Embedder 测试文档

## 测试架构概述

本测试套件基于技术设计文档实现，包含功能测试、性能测试、安全测试三个维度，采用Go语言原生测试框架和testify/assert断言库。

## 目录结构

```
test/
├── api/
│   ├── functional/          # 功能测试
│   │   ├── semantic_test.go         # 语义搜索接口测试
│   │   ├── summary_test.go          # 索引摘要接口测试
│   │   ├── create_embedding_test.go # 创建索引接口测试
│   │   └── delete_embedding_test.go # 删除索引接口测试
│   ├── performance/         # 性能测试
│   └── security/            # 安全测试
├── mocks/
│   ├── db_mock.go           # 数据库Mock
│   └── redis_mock.go        # Redis Mock
└── README.md               # 本文档
```

## 测试环境要求

### 系统要求
- Go 1.19+
- 支持的操作系统: Windows 10/11, macOS, Linux

### 依赖包
```bash
go get github.com/stretchr/testify
go get github.com/gorilla/mux
```

## 测试执行方法

### 1. 运行所有功能测试
```bash
# 在项目根目录执行
go test ./test/api/functional/... -v

# 生成覆盖率报告
go test ./test/api/functional/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### 2. 运行单个测试文件
```bash
# 运行语义搜索测试
go test ./test/api/functional -run TestSemanticSearch -v

# 运行索引摘要测试
go test ./test/api/functional -run TestIndexSummary -v

# 运行创建索引测试
go test ./test/api/functional -run TestCreateIndexing -v

# 运行删除索引测试
go test ./test/api/functional -run TestDeleteEmbedding -v
```

### 3. 运行特定测试用例
```bash
# 运行语义搜索成功场景
go test ./test/api/functional -run TestSemanticSearch_Success -v

# 运行边界条件测试
go test ./test/api/functional -run TestSemanticSearch_InvalidTopK -v
```

### 4. 并行测试执行
```bash
# 使用并行模式运行测试
go test ./test/api/functional/... -v -parallel 4
```

## 测试数据说明

### 功能测试数据
- **有效查询**: "查找用户认证相关的函数", "搜索数据库连接代码"
- **边界条件**: 空查询、超长查询(1000字符)、特殊字符
- **无效路径**: 相对路径遍历、系统敏感路径
- **并发场景**: 模拟多用户同时操作

### Mock数据配置
- 数据库Mock: 支持所有CRUD操作模拟
- Redis Mock: 支持锁机制、缓存操作模拟
- 向量存储Mock: 支持搜索、索引操作模拟

## 测试用例覆盖

### 语义搜索接口
- ✅ 成功搜索场景
- ✅ 空查询参数验证
- ✅ TopK边界条件验证
- ✅ 向量存储错误处理
- ✅ HTTP处理器测试
- ✅ 并发请求处理

### 索引摘要接口
- ✅ 成功获取摘要
- ✅ 空路径参数验证
- ✅ 索引不存在场景
- ✅ 数据库错误处理
- ✅ 空索引场景
- ✅ 大型代码库测试

### 创建索引接口
- ✅ 成功创建任务
- ✅ 路径参数验证
- ✅ 并发任务处理
- ✅ 强制重建功能
- ✅ 锁机制测试
- ✅ 数据库错误处理

### 删除索引接口
- ✅ 成功删除索引
- ✅ 路径参数验证
- ✅ 索引不存在场景
- ✅ 级联删除测试
- ✅ 部分失败处理
- ✅ 并发删除测试

## 性能测试

### 负载测试
```bash
# 使用k6进行性能测试（需安装k6）
k6 run test/api/performance/load_test.js
```

### 基准测试
```bash
# 运行Go基准测试
go test ./test/api/performance -bench=. -benchmem
```

## 安全测试

### 输入验证测试
- SQL注入防护测试
- XSS攻击防护测试
- 路径遍历攻击防护测试
- 参数边界验证测试

### 权限控制测试
- 未授权访问测试
- 权限越界测试
- Token验证测试

## 持续集成

### GitHub Actions集成
测试已集成到GitHub Actions工作流中，每次代码提交自动触发：

```yaml
# .github/workflows/api-test.yml
name: API Testing Pipeline
on: [push, pull_request]
```

### 质量门禁
- 功能测试通过率: ≥95%
- 代码覆盖率: ≥85%
- 性能测试P95响应时间: ≤2秒
- 安全扫描: 0高危漏洞

## 调试指南

### 日志调试
```bash
# 启用详细日志
go test ./test/api/functional -v -args -log-level=debug

# 输出到文件
go test ./test/api/functional -v > test.log 2>&1
```

### 断点调试
使用Delve调试器：
```bash
# 安装delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 调试单个测试
dlv test ./test/api/functional -- -test.run TestSemanticSearch_Success
```

## 常见问题

### 1. 测试失败处理
- 检查Mock期望是否设置正确
- 验证测试数据是否符合预期
- 确认依赖服务是否可用

### 2. 并发测试问题
- 使用`-parallel 1`禁用并行执行
- 检查全局状态污染
- 验证锁机制实现

### 3. 性能测试调优
- 调整并发用户数
- 优化测试数据规模
- 监控系统资源使用

## 扩展测试

### 添加新测试用例
1. 在对应测试文件中添加新函数
2. 遵循命名规范: `Test{功能}_{场景}`
3. 使用表驱动测试模式处理多场景
4. 确保Mock期望设置完整

### 集成新Mock
1. 在`test/mocks/`目录添加新Mock文件
2. 实现对应接口的所有方法
3. 在测试文件中导入并使用

## 联系支持

如有测试相关问题，请联系：
- 技术负责人: [技术团队]
- 测试文档: [技术设计文档](../docs/technical.md)
- 问题追踪: GitHub Issues
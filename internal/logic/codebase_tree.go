package logic

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

type CodebaseTreeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCodebaseTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CodebaseTreeLogic {
	return &CodebaseTreeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CodebaseTreeLogic) GetCodebaseTree(req *types.CodebaseTreeRequest) (*types.CodebaseTreeResponse, error) {
	log.Printf("[DEBUG] ===== GetCodebaseTree 开始执行 =====")
	log.Printf("[DEBUG] 请求参数: ClientId=%s, CodebasePath=%s, CodebaseName=%s, MaxDepth=%v, IncludeFiles=%v",
		req.ClientId, req.CodebasePath, req.CodebaseName, req.MaxDepth, req.IncludeFiles)

	// 参数验证
	if err := l.validateRequest(req); err != nil {
		log.Printf("[DEBUG] 参数验证失败: %v", err)
		return nil, errs.FileNotFound
	}
	log.Printf("[DEBUG] 参数验证通过")

	// 权限验证
	codebaseId, err := l.verifyCodebasePermission(req)
	if err != nil {
		log.Printf("[DEBUG] 权限验证失败: %v", err)
		return nil, errs.FileNotFound
	}
	log.Printf("[DEBUG] 权限验证通过，获得 codebaseId: %d", codebaseId)

	// 构建目录树
	log.Printf("[DEBUG] 开始构建目录树...")
	tree, err := l.buildDirectoryTree(codebaseId, req)
	if err != nil {
		log.Printf("[DEBUG] 构建目录树失败: %v", err)
		return nil, fmt.Errorf("构建目录树失败: %w", err)
	}

	log.Printf("[DEBUG] 目录树构建完成，最终结果:")
	if tree != nil {
		log.Printf("[DEBUG]   根节点名称: %s", tree.Name)
		log.Printf("[DEBUG]   根节点路径: %s", tree.Path)
		log.Printf("[DEBUG]   根节点类型: %s", tree.Type)
		log.Printf("[DEBUG]   根节点子节点数量: %d", len(tree.Children))

		// 调用独立的树结构打印函数
		l.printTreeStructure(tree)
	} else {
		log.Printf("[DEBUG] 警告: 构建的树为空")
	}

	log.Printf("[DEBUG] ===== GetCodebaseTree 执行完成 =====")
	return &types.CodebaseTreeResponse{
		Code:    0,
		Message: "ok",
		Success: true,
		Data:    tree,
	}, nil
}

func (l *CodebaseTreeLogic) validateRequest(req *types.CodebaseTreeRequest) error {
	if req.ClientId == "" {
		return fmt.Errorf("缺少必需参数: clientId")
	}
	if req.CodebasePath == "" {
		return fmt.Errorf("缺少必需参数: codebasePath")
	}
	if req.CodebaseName == "" {
		return fmt.Errorf("缺少必需参数: codebaseName")
	}
	return nil
}

func (l *CodebaseTreeLogic) verifyCodebasePermission(req *types.CodebaseTreeRequest) (int32, error) {
	// 添加调试日志
	log.Printf("[DEBUG] verifyCodebasePermission - 开始权限验证")
	log.Printf("[DEBUG] verifyCodebasePermission - ClientId: %s", req.ClientId)
	log.Printf("[DEBUG] verifyCodebasePermission - CodebasePath: %s", req.CodebasePath)
	log.Printf("[DEBUG] verifyCodebasePermission - CodebaseName: %s", req.CodebaseName)

	// 检查是否应该根据 ClientId 和 CodebasePath 从数据库查询真实的 codebaseId
	log.Printf("[DEBUG] verifyCodebasePermission - 检查数据库中是否存在匹配的 codebase 记录")

	// 尝试根据 ClientId 和 CodebasePath 查询真实的 codebase
	codebase, err := l.svcCtx.Querier.Codebase.WithContext(l.ctx).
		Where(l.svcCtx.Querier.Codebase.ClientID.Eq(req.ClientId)).
		Where(l.svcCtx.Querier.Codebase.ClientPath.Eq(req.CodebasePath)).
		First()

	if err != nil {
		log.Printf("[DEBUG] verifyCodebasePermission - 数据库查询失败或未找到匹配记录: %v", err)
		log.Printf("[DEBUG] verifyCodebasePermission - 将使用模拟的 codebaseId: 1")
		// 这里应该实现实际的权限验证逻辑
		// 由于是MVP版本，我们暂时返回一个模拟的ID
		codebaseId := int32(1)
		log.Printf("[DEBUG] verifyCodebasePermission - 返回模拟 codebaseId: %d", codebaseId)
		return codebaseId, nil
	}

	log.Printf("[DEBUG] verifyCodebasePermission - 找到匹配的 codebase 记录")
	log.Printf("[DEBUG] verifyCodebasePermission - 数据库记录 ID: %d, Name: %s, Status: %s",
		codebase.ID, codebase.Name, codebase.Status)

	log.Printf("[DEBUG] verifyCodebasePermission - 返回真实的 codebaseId: %d", codebase.ID)
	return codebase.ID, nil
}

// printTreeStructure 递归打印树结构
func (l *CodebaseTreeLogic) printTreeStructure(tree *types.TreeNode) {
	// 递归打印树结构
	var printTree func(node *types.TreeNode, indent string)
	printTree = func(node *types.TreeNode, indent string) {
		log.Printf("[DEBUG] %s├── %s (%s) - 子节点数: %d", indent, node.Name, node.Type, len(node.Children))
		for i := range node.Children {
			newIndent := indent + "│  "
			if i == len(node.Children)-1 {
				newIndent = indent + "   "
			}
			printTree(node.Children[i], newIndent)
		}
	}
	printTree(tree, "")
}

func (l *CodebaseTreeLogic) buildDirectoryTree(codebaseId int32, req *types.CodebaseTreeRequest) (*types.TreeNode, error) {
	log.Printf("[DEBUG] ===== buildDirectoryTree 开始执行 =====")
	log.Printf("[DEBUG] 输入参数: codebaseId=%d, codebasePath=%s", codebaseId, req.CodebasePath)

	// 检查数据库中是否存在该 codebaseId
	l.checkCodebaseInDatabase(codebaseId)

	// 从向量存储中获取文件路径
	records, err := l.getRecordsFromVectorStore(codebaseId, req.CodebasePath)
	if err != nil {
		return nil, err
	}

	// 分析记录并提取文件路径
	filePaths, err := l.analyzeRecordsAndExtractPaths(records)
	if err != nil {
		return nil, err
	}

	// 设置构建参数
	maxDepth, includeFiles := l.buildTreeParameters(req)

	// 构建目录树
	log.Printf("[DEBUG] ===== 关键诊断点：开始构建目录树 =====")
	log.Printf("[DEBUG] 输入到 BuildDirectoryTree 的参数:")
	log.Printf("[DEBUG]   filePaths 数量: %d", len(filePaths))
	log.Printf("[DEBUG]   maxDepth: %d", maxDepth)
	log.Printf("[DEBUG]   includeFiles: %v", includeFiles)

	result, err := BuildDirectoryTree(filePaths, maxDepth, includeFiles)
	if err != nil {
		log.Printf("[DEBUG] ❌ BuildDirectoryTree 执行失败: %v", err)
		return nil, err
	}

	log.Printf("[DEBUG] ✅ BuildDirectoryTree 执行成功")
	log.Printf("[DEBUG] ===== buildDirectoryTree 执行完成 =====")
	return result, nil
}

// checkCodebaseInDatabase 检查数据库中是否存在该 codebaseId
func (l *CodebaseTreeLogic) checkCodebaseInDatabase(codebaseId int32) {
	log.Printf("[DEBUG] 检查数据库中是否存在 codebaseId: %d", codebaseId)
	codebase, err := l.svcCtx.Querier.Codebase.WithContext(l.ctx).Where(l.svcCtx.Querier.Codebase.ID.Eq(codebaseId)).First()
	if err != nil {
		log.Printf("[DEBUG] 数据库中未找到 codebaseId %d: %v", codebaseId, err)
	} else {
		log.Printf("[DEBUG] 数据库中找到 codebase 记录 - ID: %d, Name: %s, Path: %s, Status: %s",
			codebase.ID, codebase.Name, codebase.Path, codebase.Status)
	}
}

// getRecordsFromVectorStore 从向量存储中获取文件记录
func (l *CodebaseTreeLogic) getRecordsFromVectorStore(codebaseId int32, codebasePath string) ([]*types.CodebaseRecord, error) {
	log.Printf("[DEBUG] ===== 关键诊断点：调用 GetCodebaseRecords =====")
	log.Printf("[DEBUG] 调用参数: codebaseId=%d, codebasePath=%s", codebaseId, codebasePath)

	// 检查向量存储连接状态
	log.Printf("[DEBUG] 向量存储连接状态检查...")
	log.Printf("[DEBUG] VectorStore 实例类型: %T", l.svcCtx.VectorStore)
	if l.svcCtx.VectorStore == nil {
		log.Printf("[DEBUG] ❌ VectorStore 为 nil，这是问题的根源！")
		return nil, fmt.Errorf("VectorStore 未初始化")
	}

	records, err := l.svcCtx.VectorStore.GetCodebaseRecords(l.ctx, codebaseId, codebasePath)
	if err != nil {
		log.Printf("[DEBUG] ❌ GetCodebaseRecords 调用失败: %v", err)
		log.Printf("[DEBUG] 这可能是导致只显示一级目录的根本原因：数据获取失败")
		log.Printf("[DEBUG] 错误详细信息: %+v", err)
		return nil, fmt.Errorf("查询文件路径失败: %w", err)
	}

	log.Printf("[DEBUG] ✅ GetCodebaseRecords 调用成功")
	log.Printf("[DEBUG] 返回记录数: %d", len(records))

	if len(records) == 0 {
		l.logEmptyRecordsDiagnostic(codebaseId, codebasePath)
	}

	// 合并相同文件路径的记录
	log.Printf("[DEBUG] 开始合并相同文件路径的记录...")
	mergedRecords, mergeCount := l.mergeRecordsByFilePath(records)
	log.Printf("[DEBUG] 合并完成：原始记录数=%d，合并后记录数=%d，合并了%d个重复路径",
		len(records), len(mergedRecords), mergeCount)

	// 详细诊断检查记录的结构和内容
	l.logRecordStructureAnalysis(mergedRecords)

	return mergedRecords, nil
}

// mergeRecordsByFilePath 合并相同文件路径的记录
func (l *CodebaseTreeLogic) mergeRecordsByFilePath(records []*types.CodebaseRecord) ([]*types.CodebaseRecord, int) {
	// 使用 map 按文件路径分组
	filePathMap := make(map[string][]*types.CodebaseRecord)

	for _, record := range records {
		filePathMap[record.FilePath] = append(filePathMap[record.FilePath], record)
	}

	// 合并重复路径的记录
	var mergedRecords []*types.CodebaseRecord
	mergeCount := 0

	for filePath, fileRecords := range filePathMap {
		if len(fileRecords) == 1 {
			// 没有重复，直接添加
			mergedRecords = append(mergedRecords, fileRecords[0])
		} else {
			// 有重复，合并记录
			log.Printf("[DEBUG] 合并重复文件路径: %s (共%d条记录)", filePath, len(fileRecords))
			mergedRecord := l.mergeSingleFileRecords(fileRecords)
			mergedRecords = append(mergedRecords, mergedRecord)
			mergeCount += len(fileRecords) - 1
		}
	}

	return mergedRecords, mergeCount
}

// mergeSingleFileRecords 合并单个文件的多条记录
func (l *CodebaseTreeLogic) mergeSingleFileRecords(records []*types.CodebaseRecord) *types.CodebaseRecord {
	if len(records) == 0 {
		return nil
	}

	// 以第一条记录为基础
	baseRecord := records[0]

	// 合并内容
	var mergedContent strings.Builder
	var totalTokens int
	var allRanges []int

	for _, record := range records {
		mergedContent.WriteString(record.Content)
		totalTokens += record.TokenCount
		allRanges = append(allRanges, record.Range...)
	}

	// 创建合并后的记录
	mergedRecord := &types.CodebaseRecord{
		Id:          baseRecord.Id,
		FilePath:    baseRecord.FilePath,
		Language:    baseRecord.Language,
		Content:     mergedContent.String(),
		TokenCount:  totalTokens,
		LastUpdated: baseRecord.LastUpdated,
	}

	// 合并范围信息（简单连接，可能需要更复杂的逻辑）
	if len(allRanges) > 0 {
		mergedRecord.Range = allRanges
	}

	return mergedRecord
}

// logEmptyRecordsDiagnostic 记录空记录的诊断信息
func (l *CodebaseTreeLogic) logEmptyRecordsDiagnostic(codebaseId int32, codebasePath string) {
	log.Printf("[DEBUG] ❌ 关键发现：未找到任何记录，这是导致目录树为空的直接原因！")
	log.Printf("[DEBUG] 问题根源分析:")
	log.Printf("[DEBUG] 1. codebaseId %d 在数据库中不存在", codebaseId)
	log.Printf("[DEBUG] 2. codebasePath '%s' 不匹配数据库中存储的路径", codebasePath)
	log.Printf("[DEBUG] 3. Weaviate 向量存储中没有对应的数据")
	log.Printf("[DEBUG] 4. Weaviate 连接配置错误")
	log.Printf("[DEBUG] 5. Tenant/命名空间生成错误")
	log.Printf("[DEBUG] 可能的原因:")
	log.Printf("[DEBUG] 1. codebaseId %d 在数据库中不存在", codebaseId)
	log.Printf("[DEBUG] 2. codebasePath %s 不匹配", codebasePath)
	log.Printf("[DEBUG] 3. Weaviate 中没有对应的数据")
	log.Printf("[DEBUG] 4. Weaviate 连接失败")
	log.Printf("[DEBUG] 5. Tenant 名称生成错误")
	log.Printf("[DEBUG] 6. 过滤器条件过于严格")

	// 详细诊断：检查数据库和向量存储的连接状态
	log.Printf("[DEBUG] ===== 深度诊断：数据库和向量存储状态检查 =====")

	// 1. 检查数据库连接和记录
	log.Printf("[DEBUG] 1. 数据库状态检查...")
	allCodebases, err := l.svcCtx.Querier.Codebase.WithContext(l.ctx).Find()
	if err != nil {
		log.Printf("[DEBUG] ❌ 数据库查询失败: %v", err)
	} else {
		log.Printf("[DEBUG] ✅ 数据库连接正常，共找到 %d 个 codebase 记录:", len(allCodebases))
		for i, cb := range allCodebases {
			log.Printf("[DEBUG]   Codebase %d: ID=%d, ClientID='%s', Name='%s', ClientPath='%s', Status='%s'",
				i+1, cb.ID, cb.ClientID, cb.Name, cb.ClientPath, cb.Status)
		}
	}

	// 2. 检查向量存储连接
	log.Printf("[DEBUG] 2. 向量存储状态检查...")
	log.Printf("[DEBUG]   VectorStore 类型: %T", l.svcCtx.VectorStore)
	log.Printf("[DEBUG]   VectorStore 是否为 nil: %v", l.svcCtx.VectorStore == nil)

	// 3. 尝试直接查询向量存储中的所有记录
	log.Printf("[DEBUG] 3. 尝试查询向量存储中的所有记录...")
	if l.svcCtx.VectorStore != nil {
		// 尝试使用一个空的 codebasePath 来获取所有记录
		allRecords, err := l.svcCtx.VectorStore.GetCodebaseRecords(l.ctx, codebaseId, "")
		if err != nil {
			log.Printf("[DEBUG] ❌ 查询所有向量存储记录失败: %v", err)
		} else {
			log.Printf("[DEBUG] ✅ 向量存储中总共找到 %d 条记录", len(allRecords))
			if len(allRecords) > 0 {
				log.Printf("[DEBUG]   前5条记录示例:")
				for i := 0; i < min(5, len(allRecords)); i++ {
					log.Printf("[DEBUG]     记录 %d: FilePath='%s'", i+1, allRecords[i].FilePath)
				}
			}
		}
	}

	// 4. 检查请求参数的详细情况
	log.Printf("[DEBUG] 4. 请求参数详细分析:")
	log.Printf("[DEBUG]   codebaseId: %d (类型: %T)", codebaseId, codebaseId)
	log.Printf("[DEBUG]   req.CodebasePath: '%s' (长度: %d)", codebasePath, len(codebasePath))
	log.Printf("[DEBUG]   req.CodebasePath 为空: %v", codebasePath == "")
	log.Printf("[DEBUG]   req.CodebasePath 为 '.': %v", codebasePath == ".")
}

// logRecordStructureAnalysis 记录结构分析
func (l *CodebaseTreeLogic) logRecordStructureAnalysis(records []*types.CodebaseRecord) {
	log.Printf("[DEBUG] ===== 数据流跟踪：原始记录结构检查 =====")
	if len(records) > 0 {
		for i := 0; i < min(5, len(records)); i++ {
			record := records[i]
			// 类型转换
			if record == nil {
				log.Printf("[DEBUG] 记录 %d: nil", i+1)
				continue
			}

			codebaseRecord := record

			log.Printf("[DEBUG] 记录 %d 结构分析:", i+1)
			log.Printf("[DEBUG]   记录类型: %T", record)

			// 安全地访问 CodebaseRecord 字段
			log.Printf("[DEBUG]   ID: %v", codebaseRecord.Id)
			log.Printf("[DEBUG]   FilePath: %v", codebaseRecord.FilePath)
			log.Printf("[DEBUG]   Language: %v", codebaseRecord.Language)
			log.Printf("[DEBUG]   Content 长度: %d", len(codebaseRecord.Content))
			log.Printf("[DEBUG]   Range: %v", codebaseRecord.Range)
			log.Printf("[DEBUG]   TokenCount: %v", codebaseRecord.TokenCount)
			log.Printf("[DEBUG]   LastUpdated: %v", codebaseRecord.LastUpdated)

			// 检查路径格式
			log.Printf("[DEBUG]   路径分析:")
			log.Printf("[DEBUG]     是否以/开头: %v", strings.HasPrefix(codebaseRecord.FilePath, "/"))
			log.Printf("[DEBUG]     是否包含\\: %v", strings.Contains(codebaseRecord.FilePath, "\\"))
			log.Printf("[DEBUG]     路径分段: %v", strings.Split(codebaseRecord.FilePath, "/"))
		}
	} else {
		log.Printf("[DEBUG] 没有记录可供分析")
	}
}

// analyzeRecordsAndExtractPaths 分析记录并提取文件路径
func (l *CodebaseTreeLogic) analyzeRecordsAndExtractPaths(records []*types.CodebaseRecord) ([]string, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("没有记录可供分析")
	}

	log.Printf("[DEBUG] ✅ 成功获取记录，开始分析文件路径结构...")

	// 详细诊断：分析记录的完整性和结构
	l.logDetailedRecordAnalysis(records)

	// 提取文件路径
	log.Printf("[DEBUG] ===== 关键诊断点：文件路径提取 =====")
	var filePaths []string
	for i, record := range records {
		filePaths = append(filePaths, record.FilePath)
		if i < 10 { // 增加到前10个路径以便更好分析
			log.Printf("[DEBUG] 文件路径 %d: %s", i+1, record.FilePath)
		}
	}

	if len(records) > 10 {
		log.Printf("[DEBUG] ... (还有 %d 个路径未显示)", len(records)-10)
	}

	// 添加调试：检查是否有重复的文件路径
	pathCount := make(map[string]int)
	for _, path := range filePaths {
		pathCount[path]++
	}
	log.Printf("[DEBUG] 文件路径统计:")
	log.Printf("[DEBUG]   总文件路径数: %d", len(filePaths))
	log.Printf("[DEBUG]   去重后路径数: %d", len(pathCount))

	// 分析路径深度分布
	l.analyzePathDepthDistribution(filePaths)

	return filePaths, nil
}

// logDetailedRecordAnalysis 记录详细分析
func (l *CodebaseTreeLogic) logDetailedRecordAnalysis(records []*types.CodebaseRecord) {
	log.Printf("[DEBUG] ===== 数据流跟踪：记录详细分析 =====")
	log.Printf("[DEBUG] 记录总数: %d", len(records))

	// 统计分析
	pathAnalysis := make(map[string]int)
	languageAnalysis := make(map[string]int)
	contentLengthAnalysis := make(map[string]int)

	for i, record := range records {
		// 记录基本信息
		log.Printf("[DEBUG] 记录 %d 分析:", i+1)
		log.Printf("[DEBUG]   ID: %v", record.Id)
		log.Printf("[DEBUG]   FilePath: %v", record.FilePath)
		log.Printf("[DEBUG]   Language: %v", record.Language)
		log.Printf("[DEBUG]   ContentLength: %d", len(record.Content))

		// 统计分析
		pathAnalysis[record.FilePath]++
		languageAnalysis[record.Language]++

		contentLengthCategory := "empty"
		if len(record.Content) == 0 {
			contentLengthCategory = "empty"
		} else if len(record.Content) < 100 {
			contentLengthCategory = "short"
		} else if len(record.Content) < 1000 {
			contentLengthCategory = "medium"
		} else {
			contentLengthCategory = "long"
		}
		contentLengthAnalysis[contentLengthCategory]++

		// 只显示前10个记录的详细信息
		if i < 10 {
			log.Printf("[DEBUG]   Content 预览: %q...", record.Content[:min(100, len(record.Content))])
		}
	}

	// 输出统计结果
	log.Printf("[DEBUG] ===== 数据流跟踪：统计分析 =====")
	log.Printf("[DEBUG] 唯一文件路径数: %d", len(pathAnalysis))
	log.Printf("[DEBUG] 语言分布:")
	for lang, count := range languageAnalysis {
		log.Printf("[DEBUG]   %s: %d", lang, count)
	}
	log.Printf("[DEBUG] 内容长度分布:")
	for category, count := range contentLengthAnalysis {
		log.Printf("[DEBUG]   %s: %d", category, count)
	}

	// 检查重复文件路径
	duplicatePaths := 0
	for path, count := range pathAnalysis {
		if count > 1 {
			duplicatePaths++
			log.Printf("[DEBUG] 重复文件路径: %s (出现 %d 次)", path, count)
		}
	}
	log.Printf("[DEBUG] 重复文件路径数: %d", duplicatePaths)

	// 文件路径深度分析
	log.Printf("[DEBUG] ===== 数据流跟踪：文件路径深度分析 =====")
	depthAnalysis := make(map[int]int)
	depthPathExamples := make(map[int][]string)
	for path := range pathAnalysis {
		depth := strings.Count(path, "/") + strings.Count(path, "\\")
		depthAnalysis[depth]++
		// 为每个深度保留3个示例路径
		if len(depthPathExamples[depth]) < 3 {
			depthPathExamples[depth] = append(depthPathExamples[depth], path)
		}
	}
	for depth, count := range depthAnalysis {
		log.Printf("[DEBUG] 深度 %d: %d 个文件", depth, count)
		// 显示该深度的示例路径
		for _, examplePath := range depthPathExamples[depth] {
			log.Printf("[DEBUG]   示例路径: %s", examplePath)
		}
	}

	// 显示前20个唯一文件路径作为示例
	log.Printf("[DEBUG] ===== 数据流跟踪：文件路径示例 =====")
	count := 0
	for path := range pathAnalysis {
		if count < 20 {
			log.Printf("[DEBUG]   文件路径 %d: %s", count+1, path)
			count++
		} else {
			break
		}
	}
	if len(pathAnalysis) > 20 {
		log.Printf("[DEBUG]   ... (还有 %d 个文件路径未显示)", len(pathAnalysis)-20)
	}
}

// analyzePathDepthDistribution 分析路径深度分布
func (l *CodebaseTreeLogic) analyzePathDepthDistribution(filePaths []string) {
	if len(filePaths) > 0 {
		depthCount := make(map[int]int)
		pathDepthExamples := make(map[int][]string)
		for _, path := range filePaths {
			depth := strings.Count(path, string(filepath.Separator))
			depthCount[depth]++
			if len(pathDepthExamples[depth]) < 3 { // 每个深度保留3个示例
				pathDepthExamples[depth] = append(pathDepthExamples[depth], path)
			}
		}

		log.Printf("[DEBUG] 🔍 文件路径深度分布分析:")
		for depth := 0; depth <= 10; depth++ {
			if count, exists := depthCount[depth]; exists {
				log.Printf("[DEBUG]   深度 %d: %d 个文件", depth, count)
				for _, example := range pathDepthExamples[depth] {
					log.Printf("[DEBUG]     示例: %s", example)
				}
			}
		}

		// 检查是否所有路径都是同一深度（这可能表明问题）
		if len(depthCount) == 1 {
			log.Printf("[DEBUG] ⚠️  警告: 所有文件路径都是同一深度，这可能表明数据有问题！")
		}
	}
}

// buildTreeParameters 设置构建参数
func (l *CodebaseTreeLogic) buildTreeParameters(req *types.CodebaseTreeRequest) (int, bool) {
	// 设置默认值
	maxDepth := 10 // 默认最大深度
	if req.MaxDepth != nil {
		maxDepth = *req.MaxDepth
	}

	includeFiles := true // 默认包含文件
	if req.IncludeFiles != nil {
		includeFiles = *req.IncludeFiles
	}

	log.Printf("[DEBUG] 目录树构建参数:")
	log.Printf("[DEBUG]   maxDepth: %d (请求值: %v)", maxDepth, req.MaxDepth)
	log.Printf("[DEBUG]   includeFiles: %v (请求值: %v)", includeFiles, req.IncludeFiles)

	return maxDepth, includeFiles
}

// BuildDirectoryTree 构建目录树
func BuildDirectoryTree(filePaths []string, maxDepth int, includeFiles bool) (*types.TreeNode, error) {
	log.Printf("[DEBUG] ===== BuildDirectoryTree 开始执行 =====")
	log.Printf("[DEBUG] 输入参数: filePaths数量=%d, maxDepth=%d, includeFiles=%v", len(filePaths), maxDepth, includeFiles)

	if len(filePaths) == 0 {
		log.Printf("[DEBUG] ❌ 文件路径列表为空，这是问题的直接原因！")
		return nil, fmt.Errorf("文件路径列表为空")
	}

	// 🔧 修复：在开始处理前对所有路径进行规范化
	log.Printf("[DEBUG] 🔧 修复：对所有输入路径进行规范化处理...")
	normalizedPaths := make([]string, len(filePaths))
	for i, path := range filePaths {
		normalizedPaths[i] = normalizePath(path)
		if i < 10 { // 只显示前10个避免日志过多
			log.Printf("[DEBUG]   路径规范化 %d: '%s' -> '%s'", i+1, path, normalizedPaths[i])
		}
	}
	filePaths = normalizedPaths

	// 添加诊断日志：显示规范化后的文件路径列表
	log.Printf("[DEBUG] 🔍 规范化后的文件路径列表分析 (共 %d 个):", len(filePaths))
	for i, path := range filePaths {
		if i < 10 { // 只显示前10个避免日志过多
			log.Printf("[DEBUG]   规范化路径 %d: %s", i+1, path)
		}
		if i == 10 {
			log.Printf("[DEBUG]   ... (还有 %d 个路径未显示)", len(filePaths)-10)
		}
	}

	// 对文件路径进行去重处理
	uniquePaths := make([]string, 0)
	pathSet := make(map[string]bool)
	duplicateCount := 0

	for _, path := range filePaths {
		if !pathSet[path] {
			pathSet[path] = true
			uniquePaths = append(uniquePaths, path)
		} else {
			duplicateCount++
		}
	}

	// 添加诊断日志：显示去重结果
	log.Printf("[DEBUG] BuildDirectoryTree - 路径去重结果:")
	log.Printf("[DEBUG]   规范化路径总数: %d", len(filePaths))
	log.Printf("[DEBUG]   重复路径数: %d", duplicateCount)
	log.Printf("[DEBUG]   去重后路径数: %d", len(uniquePaths))

	log.Printf("[DEBUG] BuildDirectoryTree - 去重后的文件路径列表:")
	for i, path := range uniquePaths {
		if i < 10 { // 只显示前10个避免日志过多
			log.Printf("[DEBUG]   唯一路径 %d: %s", i+1, path)
		}
		if i == 10 && len(uniquePaths) > 10 {
			log.Printf("[DEBUG]   ... (还有 %d 个路径未显示)", len(uniquePaths)-10)
		}
	}

	// 使用去重后的路径列表
	filePaths = uniquePaths

	// 提取根路径
	rootPath := extractRootPath(filePaths)

	// 🔧 修复：确保根路径也被规范化
	rootPath = normalizePath(rootPath)

	// 添加诊断日志：显示提取的根路径
	log.Printf("[DEBUG] BuildDirectoryTree - 提取的根路径: '%s'", rootPath)

	// 处理根路径为空的情况
	if rootPath == "" {
		log.Printf("[DEBUG] 根路径为空，使用默认根目录 '.'")
		rootPath = "."
	}

	// 🔧 修复：确保根路径规范化
	rootPath = normalizePath(rootPath)

	root := &types.TreeNode{
		Name:     filepath.Base(rootPath),
		Path:     rootPath,
		Type:     "directory",
		Children: make([]*types.TreeNode, 0),
	}

	// 添加诊断日志：显示根节点信息
	log.Printf("[DEBUG] 创建根节点 - Name: '%s', Path: '%s'", root.Name, root.Path)

	pathMap := make(map[string]*types.TreeNode)
	pathMap[root.Path] = root

	// 添加调试：跟踪文件处理过程
	processedFiles := make(map[string]int)
	skippedFiles := 0
	processedFilesCount := 0

	log.Printf("[DEBUG] 开始处理文件路径列表，总数: %d", len(filePaths))
	log.Printf("[DEBUG] 配置参数 - includeFiles: %v, maxDepth: %d", includeFiles, maxDepth)

	for _, filePath := range filePaths {
		// 添加调试：跟踪每个文件路径的处理
		processedFiles[filePath]++
		log.Printf("[DEBUG] 处理文件路径: %s (第 %d 次处理)", filePath, processedFiles[filePath])

		if !includeFiles && !isDirectory(filePath) {
			log.Printf("[DEBUG] 跳过文件 (不包含文件): %s", filePath)
			skippedFiles++
			continue
		}

		// 计算文件深度 - 添加详细的深度计算日志
		// 关键修复：处理 rootPath 为 "." 的情况
		log.Printf("[DEBUG] 🔍 关键诊断：RelativePath 计算前分析")
		log.Printf("[DEBUG]   FilePath: '%s', RootPath: '%s', len(RootPath): %d", filePath, rootPath, len(rootPath))
		log.Printf("[DEBUG]   RootPath == '.': %v", rootPath == ".")

		var relativePath string
		if rootPath == "." {
			// 当根路径为 "." 时，不应该去掉任何字符
			relativePath = filePath
			log.Printf("[DEBUG] ✅ 检测到根路径为 '.'，使用完整文件路径作为相对路径")
		} else {
			// 原有逻辑：去掉根路径部分
			relativePath = filePath[len(rootPath):]
			log.Printf("[DEBUG] ✅ 使用原有逻辑计算相对路径")
		}

		if len(relativePath) > 0 && (relativePath[0] == '/' || relativePath[0] == '\\') {
			relativePath = relativePath[1:] // 移除开头的分隔符
			log.Printf("[DEBUG] ✅ 移除开头的分隔符，新的相对路径: '%s'", relativePath)
		}

		currentDepth := strings.Count(relativePath, string(filepath.Separator))
		log.Printf("[DEBUG] 深度计算 - FilePath: '%s', RootPath: '%s', RelativePath: '%s', Depth: %d",
			filePath, rootPath, relativePath, currentDepth)

		if maxDepth > 0 && currentDepth > maxDepth {
			log.Printf("[DEBUG] 跳过文件 (超过最大深度): %s, 深度: %d, 最大深度: %d", filePath, currentDepth, maxDepth)
			skippedFiles++
			continue
		}

		// 🔧 修复：确保所有路径都使用规范化格式
		// 构建路径节点
		dir := normalizePath(filepath.Dir(filePath))
		parentPath := dir

		// 添加诊断日志：显示文件路径分析
		log.Printf("[DEBUG] ===== 数据流跟踪：文件路径处理 =====")
		log.Printf("[DEBUG] 文件路径分析 - FilePath: '%s', RootPath: '%s', Dir: '%s'", filePath, rootPath, dir)
		log.Printf("[DEBUG] 路径分割符检查 - 系统分隔符: '%s', FilePath中使用分隔符: %v",
			string(filepath.Separator), strings.Contains(filePath, "\\"))

		// 🔧 修复：路径规范化分析（现在所有路径都已规范化）
		log.Printf("[DEBUG] 规范化路径: '%s' (所有路径已统一格式)", filePath)

		// 路径组件分析
		pathComponents := strings.Split(filePath, string(filepath.Separator))
		log.Printf("[DEBUG] 路径组件分解: %v (共 %d 个组件)", pathComponents, len(pathComponents))

		// 检查根路径匹配（现在都使用规范化路径）
		if strings.HasPrefix(filePath, rootPath) {
			log.Printf("[DEBUG] ✅ 文件路径以根路径开头，应该被包含在树中")
		} else {
			log.Printf("[DEBUG] ⚠️  文件路径不以根路径开头，可能被过滤掉")
			log.Printf("[DEBUG]   根路径: '%s', 文件路径: '%s'", rootPath, filePath)
		}

		// 🔧 修复：使用规范化路径进行循环条件检查
		log.Printf("[DEBUG] 开始父路径循环 - ParentPath: '%s', Root.Path: '%s', RootPath: '%s'",
			parentPath, root.Path, rootPath)

		// 跟踪父路径构建过程
		parentPathHistory := []string{parentPath}
		log.Printf("[DEBUG] 初始化 parentPathHistory: %v", parentPathHistory)
		for parentPath != root.Path && !(rootPath == "." && parentPath == ".") && parentPath != "/" {
			log.Printf("[DEBUG] 循环处理父路径: %s", parentPath)
			if _, exists := pathMap[parentPath]; !exists {
				log.Printf("[DEBUG] 创建目录节点: %s", parentPath)
				node := &types.TreeNode{
					Name:     filepath.Base(parentPath),
					Path:     parentPath, // 🔧 修复：使用规范化路径
					Type:     "directory",
					Children: make([]*types.TreeNode, 0),
				}
				pathMap[parentPath] = node

				// 添加到父节点
				parentDirPath := normalizePath(filepath.Dir(parentPath))
				if parent, exists := pathMap[parentDirPath]; exists {
					parent.Children = append(parent.Children, node)
					log.Printf("[DEBUG] 将目录 %s 添加到父目录 %s", parentPath, parentDirPath)
				} else {
					log.Printf("[DEBUG] 警告: 父目录 %s 不存在，无法将目录 %s 添加到父目录", parentDirPath, parentPath)
				}
			} else {
				log.Printf("[DEBUG] 目录节点已存在: %s", parentPath)
			}

			// 更新父路径历史记录 - 诊断：检查是否应该更新parentPathHistory
			oldParentPath := parentPath
			parentPath = normalizePath(filepath.Dir(parentPath)) // 🔧 修复：确保父路径也规范化
			log.Printf("[DEBUG] 父路径更新: %s -> %s", oldParentPath, parentPath)

			// 诊断：检查是否应该将新父路径添加到历史记录中
			log.Printf("[DEBUG] 当前 parentPathHistory: %v", parentPathHistory)
			log.Printf("[DEBUG] 是否应该将 %s 添加到 parentPathHistory?", parentPath)
		}

		// 诊断：检查循环结束后parentPathHistory的状态
		log.Printf("[DEBUG] 循环结束后的 parentPathHistory: %v (长度: %d)", parentPathHistory, len(parentPathHistory))

		// 添加文件节点
		if includeFiles && !isDirectory(filePath) {
			processedFilesCount++
			log.Printf("[DEBUG] 处理文件节点 #%d: %s", processedFilesCount, filePath)

			fileNode, err := createFileNode(filePath)
			if err != nil {
				log.Printf("[DEBUG] 创建文件节点失败: %s, 错误: %v", filePath, err)
				continue
			}

			// 🔍 关键诊断：详细的文件节点创建信息
			log.Printf("[DEBUG] 🔍 文件节点创建详情 - 详细分析:")
			log.Printf("[DEBUG]   原始文件路径: '%s'", filePath)
			log.Printf("[DEBUG]   节点名称: '%s'", fileNode.Name)
			log.Printf("[DEBUG]   节点路径: '%s'", fileNode.Path)
			log.Printf("[DEBUG]   节点类型: '%s'", fileNode.Type)

			// 🔍 路径规范化分析 - 使用 normalizePath 函数
			normalizedFilePath := normalizePath(filePath)
			normalizedNodePath := normalizePath(fileNode.Path)
			log.Printf("[DEBUG]   normalizePath 文件路径: '%s'", normalizedFilePath)
			log.Printf("[DEBUG]   normalizePath 节点路径: '%s'", normalizedNodePath)

			// 🔍 额外诊断： filepath.Clean vs normalizePath
			cleanedFilePath := filepath.Clean(filePath)
			cleanedNodePath := filepath.Clean(fileNode.Path)
			log.Printf("[DEBUG]   filepath.Clean 文件路径: '%s'", cleanedFilePath)
			log.Printf("[DEBUG]   filepath.Clean 节点路径: '%s'", cleanedNodePath)

			// 🔍 路径格式分析
			log.Printf("[DEBUG]   原始路径包含 /: %v", strings.Contains(filePath, "/"))
			log.Printf("[DEBUG]   原始路径包含 \\: %v", strings.Contains(filePath, "\\"))
			log.Printf("[DEBUG]   节点路径包含 /: %v", strings.Contains(fileNode.Path, "/"))
			log.Printf("[DEBUG]   节点路径包含 \\: %v", strings.Contains(fileNode.Path, "\\"))
			log.Printf("[DEBUG]   normalizePath 后包含 /: %v", strings.Contains(normalizedNodePath, "/"))
			log.Printf("[DEBUG]   normalizePath 后包含 \\: %v", strings.Contains(normalizedNodePath, "\\"))

			// �� 关键诊断：检查路径一致性
			log.Printf("[DEBUG]   路径一致性检查:")
			log.Printf("[DEBUG]     原始路径 == 节点路径: %v", filePath == fileNode.Path)
			log.Printf("[DEBUG]     normalizePath(原始) == normalizePath(节点): %v", normalizedFilePath == normalizedNodePath)
			log.Printf("[DEBUG]     filepath.Clean(原始) == filepath.Clean(节点): %v", cleanedFilePath == cleanedNodePath)

			//  修复：使用规范化路径进行父目录查找
			// 添加诊断日志：显示父目录查找过程
			log.Printf("[DEBUG] 查找父目录 - Dir: '%s', RootPath: '%s', Dir == RootPath: %v", dir, rootPath, dir == rootPath)
			log.Printf("[DEBUG] pathMap 中的目录数量: %d", len(pathMap))
			for path, parentNode := range pathMap {
				log.Printf("[DEBUG]   pathMap 包含目录: '%s' (已规范化)", path)
				log.Printf("[DEBUG]   pathMap 包含目录: '%s' (已规范化)", parentNode.Name)
				log.Printf("[DEBUG]   pathMap 包含目录: '%s' (已规范化)", parentNode.Type)
			}

			// 🔧 修复：简化父目录查找逻辑（现在所有路径都已规范化）
			parentFound := false
			var foundParentNode *types.TreeNode
			var matchedParentPath string

			// 🔍 关键诊断：规范化父目录路径
			normalizedDir := normalizePath(dir)
			log.Printf("[DEBUG] 🔍 父目录查找诊断 - 规范化处理:")
			log.Printf("[DEBUG]   原始父目录路径: '%s'", dir)
			log.Printf("[DEBUG]   规范化父目录路径: '%s'", normalizedDir)
			log.Printf("[DEBUG]   pathMap 中的路径数量: %d", len(pathMap))

			for path, parentNode := range pathMap {
				log.Printf("[DEBUG] 🔍 父目录匹配诊断:")
				log.Printf("[DEBUG]   比较路径 - pathMap中的路径: '%s'", path)
				log.Printf("[DEBUG]   比较路径 - 规范化父目录: '%s'", normalizedDir)
				log.Printf("[DEBUG]   直接比较结果: %v", path == normalizedDir)

				// parentNode.Size = 20000

				if path == normalizedDir { // 🔧 修复：直接比较规范化路径
					foundParentNode = parentNode
					matchedParentPath = path
					parentFound = true
					log.Printf("[DEBUG] ✅ 找到匹配的父目录: '%s'", path)
					break
				}
			}
			if parentFound && foundParentNode != nil {
				// 将文件节点添加到找到的父目录
				// foundParentNode.Size = 100000
				if matchedParentPath == "code" {
					log.Printf("[DEBUG] ==============================================================")
					foundParentNode.Size = 10086
				}
				foundParentNode.Children = append(foundParentNode.Children, fileNode)
				log.Printf("[DEBUG] ✅ 通过规范化路径匹配将文件 %s 添加到目录 %s", filePath, matchedParentPath)
				log.Printf("[DEBUG]   目录 %s 现在有 %d 个子节点", matchedParentPath, len(foundParentNode.Children))

				log.Printf("[DEBUG]   目录 %s子节点", foundParentNode.Name)

				// 🌳 调试：添加文件节点后打印当前树结构
				log.Printf("[DEBUG] 🌳 ===== 文件添加后树结构调试 =====")
				log.Printf("[DEBUG] 🌳 新增文件: %s", filePath)
				log.Printf("[DEBUG] 🌳 位置: %s/%s", matchedParentPath, fileNode.Name)

				// 打印从根节点到新文件的完整路径
				var printPathToNode func(*types.TreeNode, string) string
				printPathToNode = func(node *types.TreeNode, targetPath string) string {
					if node.Path == targetPath {
						return node.Name
					}

					for _, child := range node.Children {
						result := printPathToNode(child, targetPath)
						if result != "" {
							return node.Name + "/" + result
						}
					}
					return ""
				}

				fullPath := printPathToNode(root, filePath)
				if fullPath != "" {
					log.Printf("[DEBUG] 🌳 完整路径: /%s", fullPath)
				}

				// 打印该文件的父目录子节点列表
				log.Printf("[DEBUG] 🌳 父目录 %s 的子节点列表:", matchedParentPath)
				for i, child := range foundParentNode.Children {
					log.Printf("[DEBUG] 🌳   子节点 %d: %s (%s) - 类型: %s", i+1, child.Name, child.Path, child.Type)
				}

				// 打印当前树的关键统计信息
				var countNodes func(*types.TreeNode) (int, int)
				countNodes = func(node *types.TreeNode) (int, int) {
					fileCount := 0
					dirCount := 0
					if node.Type == "file" {
						fileCount = 1
					} else {
						dirCount = 1
					}
					for _, child := range node.Children {
						f, d := countNodes(child)
						fileCount += f
						dirCount += d
					}
					return fileCount, dirCount
				}

				fileCount, dirCount := countNodes(root)
				log.Printf("[DEBUG] 🌳 当前树统计: %d 个文件, %d 个目录", fileCount, dirCount)

				// 🌳 调用 printTreeStructure 打印完整的树结构
				log.Printf("[DEBUG] 🌳 ===== 调用 printTreeStructure 打印完整树结构 =====")
				printTreeStructure(root)
			} else {
				// 🔍 关键诊断：父目录查找失败的详细分析
				log.Printf("[DEBUG] ❌ 父目录查找失败诊断:")
				log.Printf("[DEBUG]   文件路径: '%s'", filePath)
				log.Printf("[DEBUG]   期望的父目录: '%s'", dir)

				// 🔧 修复：简化根目录匹配逻辑（现在所有路径都已规范化）
				log.Printf("[DEBUG] 🔍 根目录匹配诊断:")
				log.Printf("[DEBUG]   比较路径: '%s' vs '%s'", dir, rootPath)
				log.Printf("[DEBUG]   路径是否相等: %v", dir == rootPath)

				if dir == rootPath { // 🔧 修复：直接比较规范化路径
					log.Printf("[DEBUG] 直接将文件 %s 添加到根目录 %s", filePath, rootPath)
					root.Children = append(root.Children, fileNode)
					log.Printf("[DEBUG] 根目录现在有 %d 个子节点", len(root.Children))

					// 🌳 调试：添加文件节点到根目录后打印当前树结构
					log.Printf("[DEBUG] 🌳 ===== 文件添加到根目录后树结构调试 =====")
					log.Printf("[DEBUG] 🌳 新增文件: %s", filePath)
					log.Printf("[DEBUG] 🌳 位置: 根目录/%s", fileNode.Name)

					// 打印根目录子节点列表
					log.Printf("[DEBUG] 🌳 根目录子节点列表:")
					for i, child := range root.Children {
						log.Printf("[DEBUG] 🌳   子节点 %d: %s (%s) - 类型: %s", i+1, child.Name, child.Path, child.Type)
					}

					// 打印当前树的关键统计信息
					var countNodes func(*types.TreeNode) (int, int)
					countNodes = func(node *types.TreeNode) (int, int) {
						fileCount := 0
						dirCount := 0
						if node.Type == "file" {
							fileCount = 1
						} else {
							dirCount = 1
						}
						for _, child := range node.Children {
							f, d := countNodes(child)
							fileCount += f
							dirCount += d
						}
						return fileCount, dirCount
					}

					fileCount, dirCount := countNodes(root)
					log.Printf("[DEBUG] 🌳 当前树统计: %d 个文件, %d 个目录", fileCount, dirCount)
				} else {
					// 🔍 关键诊断：父目录不存在时的详细分析
					log.Printf("[DEBUG] ❌ 父目录不存在，创建新目录: %s", dir)
					log.Printf("[DEBUG]   诊断信息:")
					log.Printf("[DEBUG]     期望父目录: '%s'", dir)
					log.Printf("[DEBUG]     根目录路径: '%s'", rootPath)
					log.Printf("[DEBUG]     dir类型判断: %v", isDirectory(dir))
					log.Printf("[DEBUG]     pathMap 中的所有路径:")
					for path := range pathMap {
						log.Printf("[DEBUG]       '%s'", path)
					}

					parentDir := &types.TreeNode{
						Name:     filepath.Base(dir),
						Path:     dir, // 🔧 修复：使用规范化路径
						Type:     "directory",
						Children: []*types.TreeNode{fileNode},
					}
					pathMap[dir] = parentDir
					root.Children = append(root.Children, parentDir)
					log.Printf("[DEBUG] 创建目录 %s 并添加文件 %s，根目录现在有 %d 个子节点", dir, filePath, len(root.Children))

					// 🌳 调试：创建新目录并添加文件后打印当前树结构
					log.Printf("[DEBUG] 🌳 ===== 创建新目录并添加文件后树结构调试 =====")
					log.Printf("[DEBUG] 🌳 新增目录: %s", dir)
					log.Printf("[DEBUG] 🌳 新增文件: %s", filePath)
					log.Printf("[DEBUG] 🌳 位置: %s/%s", dir, fileNode.Name)

					// 打印新创建的目录信息
					log.Printf("[DEBUG] 🌳 新创建目录信息:")
					log.Printf("[DEBUG] 🌳   目录名称: '%s'", parentDir.Name)
					log.Printf("[DEBUG] 🌳   目录路径: '%s'", parentDir.Path)
					log.Printf("[DEBUG] 🌳   目录类型: '%s'", parentDir.Type)
					log.Printf("[DEBUG] 🌳   目录子节点数: %d", len(parentDir.Children))

					// 打印根目录子节点列表
					log.Printf("[DEBUG] 🌳 根目录子节点列表:")
					for i, child := range root.Children {
						log.Printf("[DEBUG] 🌳   子节点 %d: %s (%s) - 类型: %s, 子节点数: %d", i+1, child.Name, child.Path, child.Type, len(child.Children))
					}

					// 打印当前树的关键统计信息
					var countNodes func(*types.TreeNode) (int, int)
					countNodes = func(node *types.TreeNode) (int, int) {
						fileCount := 0
						dirCount := 0
						if node.Type == "file" {
							fileCount = 1
						} else {
							dirCount = 1
						}
						for _, child := range node.Children {
							f, d := countNodes(child)
							fileCount += f
							dirCount += d
						}
						return fileCount, dirCount
					}

					fileCount, dirCount := countNodes(root)
					log.Printf("[DEBUG] 🌳 当前树统计: %d 个文件, %d 个目录", fileCount, dirCount)
				}
			}
		}
	}

	// 添加调试：总结处理结果
	log.Printf("[DEBUG] 目录树构建完成:")
	log.Printf("[DEBUG]   总共处理的文件路径数: %d", len(filePaths))
	log.Printf("[DEBUG]   跳过的文件数: %d", skippedFiles)
	log.Printf("[DEBUG]   实际处理的文件数: %d", processedFilesCount)
	log.Printf("[DEBUG]   pathMap 中的节点数: %d", len(pathMap))
	log.Printf("[DEBUG]   根目录的子节点数: %d", len(root.Children))

	// 详细输出根目录的子节点信息
	for i, child := range root.Children {
		log.Printf("[DEBUG]   根目录子节点 %d: %s (%s), 类型: %s, 子节点数: %d",
			i+1, child.Name, child.Path, child.Type, len(child.Children))

		// 递归输出子节点的详细信息
		if len(child.Children) > 0 {
			for j, grandChild := range child.Children {
				log.Printf("[DEBUG]     子目录 %s 的子节点 %d: %s (%s), 类型: %s",
					child.Name, j+1, grandChild.Name, grandChild.Path, grandChild.Type)
			}
		}
	}

	// 🔧 修复：使用规范化路径检查文件是否在树中
	missingFiles := 0
	for _, filePath := range filePaths {
		// 检查文件是否在树中
		var checkNode func(*types.TreeNode) bool
		var foundNodePath string
		checkNode = func(node *types.TreeNode) bool {
			// 🔧 修复：现在所有路径都已规范化，直接比较即可
			log.Printf("[DEBUG] 🔍 路径比较诊断 (修复后):")
			log.Printf("[DEBUG]   文件路径: '%s'", filePath)
			log.Printf("[DEBUG]   节点路径: '%s'", node.Path)
			log.Printf("[DEBUG]   直接比较结果: %v", node.Path == filePath)

			// 🔍 新增诊断：规范化比较
			normalizedFilePath := normalizePath(filePath)
			normalizedNodePath := normalizePath(node.Path)
			log.Printf("[DEBUG]   规范化文件路径: '%s'", normalizedFilePath)
			log.Printf("[DEBUG]   规范化节点路径: '%s'", normalizedNodePath)
			log.Printf("[DEBUG]   规范化比较结果: %v", normalizedNodePath == normalizedFilePath)

			// 🔍 关键修复：尝试多种路径匹配方式
			// 方式1：直接比较
			if node.Path == filePath {
				foundNodePath = node.Path
				log.Printf("[DEBUG] ✅ 方式1成功：直接路径匹配")
				return true
			}

			// 方式2：规范化比较
			if normalizedNodePath == normalizedFilePath {
				foundNodePath = node.Path
				log.Printf("[DEBUG] ✅ 方式2成功：规范化路径匹配")
				return true
			}

			// 方式3：尝试将 / 转换为 \ 进行比较
			slashConvertedPath := strings.ReplaceAll(filePath, "/", "\\")
			if node.Path == slashConvertedPath {
				foundNodePath = node.Path
				log.Printf("[DEBUG] ✅ 方式3成功：正斜杠转换匹配")
				return true
			}

			// 方式4：尝试将 \ 转换为 / 进行比较
			backslashConvertedPath := strings.ReplaceAll(filePath, "\\", "/")
			if node.Path == backslashConvertedPath {
				foundNodePath = node.Path
				log.Printf("[DEBUG] ✅ 方式4成功：反斜杠转换匹配")
				return true
			}

			// 方式5：使用 filepath.Clean 比较
			cleanedFilePath := filepath.Clean(filePath)
			cleanedNodePath := filepath.Clean(node.Path)
			if cleanedNodePath == cleanedFilePath {
				foundNodePath = node.Path
				log.Printf("[DEBUG] ✅ 方式5成功：filepath.Clean 匹配")
				return true
			}

			// 🔍 特别处理：对于 code/rtx4090-pods.py 文件，添加详细诊断
			if strings.Contains(filePath, "rtx4090-pods.py") {
				log.Printf("[DEBUG] 🔥 关键诊断：处理 rtx4090-pods.py 文件")
				log.Printf("[DEBUG]   原始文件路径: '%s'", filePath)
				log.Printf("[DEBUG]   节点路径: '%s'", node.Path)
				log.Printf("[DEBUG]   规范化文件路径: '%s'", normalizedFilePath)
				log.Printf("[DEBUG]   规范化节点路径: '%s'", normalizedNodePath)
				log.Printf("[DEBUG]   斜杠转换路径: '%s'", slashConvertedPath)
				log.Printf("[DEBUG]   反斜杠转换路径: '%s'", backslashConvertedPath)
				log.Printf("[DEBUG]   Cleaned 文件路径: '%s'", cleanedFilePath)
				log.Printf("[DEBUG]   Cleaned 节点路径: '%s'", cleanedNodePath)
			}

			for _, child := range node.Children {
				if checkNode(child) {
					return true
				}
			}
			return false
		}

		fileFound := checkNode(root)
		if !fileFound && includeFiles && !isDirectory(filePath) {
			missingFiles++
			log.Printf("[DEBUG] ❌ 警告: 文件路径在树中未找到: %s", filePath)
			log.Printf("[DEBUG]   诊断信息:")
			log.Printf("[DEBUG]     路径: '%s'", filePath)
			log.Printf("[DEBUG]     路径长度: %d", len(filePath))
			log.Printf("[DEBUG]     包含 /: %v", strings.Contains(filePath, "/"))
			log.Printf("[DEBUG]     包含 \\: %v", strings.Contains(filePath, "\\"))
			log.Printf("[DEBUG]     可能原因: 路径格式不匹配或文件未被正确添加到树中")

			// 🔍 新增：对于丢失的文件，显示树中的所有文件路径以便对比
			log.Printf("[DEBUG] 🔍 树中现有文件路径列表:")
			var listAllFiles func(*types.TreeNode, string)
			listAllFiles = func(n *types.TreeNode, indent string) {
				if n.Type == "file" {
					log.Printf("[DEBUG] %s  文件: '%s'", indent, n.Path)
				} else {
					for _, child := range n.Children {
						listAllFiles(child, indent+"  ")
					}
				}
			}
			listAllFiles(root, "")
		} else if fileFound {
			log.Printf("[DEBUG] ✅ 文件 %s 在树中找到，匹配的节点路径: '%s'", filePath, foundNodePath)
		} else {
			log.Printf("[DEBUG] ℹ️ 文件 %s 跳过检查 (includeFiles=%v, isDirectory=%v)", filePath, includeFiles, isDirectory(filePath))
		}
		log.Printf("[DEBUG] 文件 %s 在树中: %v", filePath, fileFound)
	}

	log.Printf("[DEBUG]   未在树中找到的文件数: %d", missingFiles)

	return root, nil
}

// extractRootPath 提取根路径
func extractRootPath(filePaths []string) string {
	log.Printf("[DEBUG] ===== extractRootPath 开始执行 =====")
	log.Printf("[DEBUG] 输入文件路径数量: %d", len(filePaths))

	if len(filePaths) == 0 {
		log.Printf("[DEBUG] ❌ 关键诊断：文件路径列表为空，这是目录树构建失败的根本原因！")
		log.Printf("[DEBUG] 问题分析:")
		log.Printf("[DEBUG] 1. GetCodebaseRecords 没有返回任何记录")
		log.Printf("[DEBUG] 2. 向量存储中可能没有数据")
		log.Printf("[DEBUG] 3. codebaseId 或 codebasePath 参数错误")
		return ""
	}

	// 🔧 修复：显示所有规范化后的文件路径以便分析
	log.Printf("[DEBUG] 🔍 关键诊断：分析所有输入文件路径 (已规范化):")
	for i, path := range filePaths {
		if i < 15 { // 增加到前15个以便更好分析
			log.Printf("[DEBUG]   路径 %d: '%s' (长度: %d)", i+1, path, len(path))
			// 检查路径格式
			log.Printf("[DEBUG]     路径分析 - 以/开头: %v, 以\\开头: %v",
				strings.HasPrefix(path, "/"), strings.HasPrefix(path, "\\"))
		}
		if i == 15 && len(filePaths) > 15 {
			log.Printf("[DEBUG]   ... (还有 %d 个路径未显示)", len(filePaths)-15)
		}
	}

	// 分析路径深度分布（使用规范化后的路径）
	depthAnalysis := make(map[int]int)
	for _, path := range filePaths {
		depth := strings.Count(path, string(filepath.Separator))
		depthAnalysis[depth]++
	}
	log.Printf("[DEBUG] 路径深度分布:")
	for depth, count := range depthAnalysis {
		log.Printf("[DEBUG]   深度 %d: %d 个路径", depth, count)
	}

	// 🔧 修复：找到所有路径的公共前缀（使用规范化后的路径）
	commonPrefix := filePaths[0]
	log.Printf("[DEBUG] 初始公共前缀（第一个路径）: '%s'", commonPrefix)

	for i, path := range filePaths[1:] {
		log.Printf("[DEBUG] 处理路径 %d: '%s'", i+2, path)
		log.Printf("[DEBUG] 当前公共前缀: '%s'", commonPrefix)

		newPrefix := findCommonPrefix(commonPrefix, path)
		log.Printf("[DEBUG] 计算得到的新公共前缀: '%s'", newPrefix)

		commonPrefix = newPrefix
		if commonPrefix == "" {
			log.Printf("[DEBUG] ⚠️ 公共前缀为空，中断查找")
			break
		}
	}

	log.Printf("[DEBUG] 最终公共前缀: '%s'", commonPrefix)

	// 🔧 修复：如果公共前缀不以目录分隔符结尾，找到最后一个分隔符
	lastSeparator := strings.LastIndexAny(commonPrefix, string(filepath.Separator))
	log.Printf("[DEBUG] 最后一个分隔符位置: %d", lastSeparator)

	if lastSeparator == -1 {
		log.Printf("[DEBUG] ❌ 关键诊断：未找到目录分隔符")
		log.Printf("[DEBUG] 问题分析:")
		log.Printf("[DEBUG] 1. 所有文件路径可能都在同一目录下（没有共同的父目录）")
		log.Printf("[DEBUG] 2. 文件路径格式可能不正确（缺少目录结构）")
		log.Printf("[DEBUG] 3. 输入的文件路径可能都是相对路径且没有共同的父目录")

		// 🔧 修复：当没有找到目录分隔符时，返回 "." 作为根路径
		// 这表示所有文件都在当前目录下
		log.Printf("[DEBUG] ✅ 关键修复：未找到目录分隔符，使用当前目录 '.' 作为根路径")
		return "."
	}

	rootPath := commonPrefix[:lastSeparator+1]
	log.Printf("[DEBUG] ✅ 提取的根路径: '%s'", rootPath)
	log.Printf("[DEBUG] 根路径长度: %d", len(rootPath))
	log.Printf("[DEBUG] ===== extractRootPath 执行完成 =====")
	return rootPath
}

// findCommonPrefix 找到两个路径的公共前缀
func findCommonPrefix(path1, path2 string) string {
	parts1 := strings.Split(path1, string(filepath.Separator))
	parts2 := strings.Split(path2, string(filepath.Separator))

	var commonParts []string
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		if parts1[i] == parts2[i] {
			commonParts = append(commonParts, parts1[i])
		} else {
			break
		}
	}

	return strings.Join(commonParts, string(filepath.Separator))
}

// isDirectory 判断路径是否为目录
func isDirectory(path string) bool {
	// 简单实现：根据路径末尾是否有分隔符判断
	return strings.HasSuffix(path, string(filepath.Separator)) || strings.HasSuffix(path, "/")
}

// normalizePath 统一路径格式，确保所有路径都使用系统标准分隔符
func normalizePath(path string) string {
	// 首先使用 filepath.Clean 进行基本规范化
	cleaned := filepath.Clean(path)

	// 确保路径使用系统标准的分隔符
	// 在 Windows 上，这会将 / 转换为 \
	// 在 Unix 上，这会将 \ 转换为 /
	return filepath.FromSlash(cleaned)
}

// createFileNode 创建文件节点
func createFileNode(filePath string) (*types.TreeNode, error) {
	// 🔧 修复：使用统一的路径规范化
	normalizedPath := normalizePath(filePath)

	//  关键诊断：文件节点创建时的路径分析
	log.Printf("[DEBUG] 🔍 createFileNode 路径分析:")
	log.Printf("[DEBUG]   输入文件路径: '%s'", filePath)
	log.Printf("[DEBUG]   规范化后路径: '%s'", normalizedPath)
	log.Printf("[DEBUG]   路径长度: %d -> %d", len(filePath), len(normalizedPath))
	log.Printf("[DEBUG]   包含 /: %v -> %v", strings.Contains(filePath, "/"), strings.Contains(normalizedPath, "/"))
	log.Printf("[DEBUG]   包含 \\: %v -> %v", strings.Contains(filePath, "\\"), strings.Contains(normalizedPath, "\\"))
	log.Printf("[DEBUG]   文件名: '%s'", filepath.Base(normalizedPath))

	// 模拟文件信息
	now := time.Now()
	node := &types.TreeNode{
		Name:         filepath.Base(normalizedPath),
		Path:         normalizedPath, // 🔧 修复：使用规范化后的路径
		Type:         "file",
		Size:         1024, // 模拟文件大小
		LastModified: &now,
	}

	// 🔍 创建后的节点信息诊断
	log.Printf("[DEBUG] 🔍 创建的文件节点信息:")
	log.Printf("[DEBUG]   节点名称: '%s'", node.Name)
	log.Printf("[DEBUG]   节点路径: '%s'", node.Path)
	log.Printf("[DEBUG]   节点类型: '%s'", node.Type)
	log.Printf("[DEBUG]   路径是否修改: %v", node.Path != filePath)

	return node, nil
}

// printTreeStructure 递归打印树结构
func printTreeStructure(tree *types.TreeNode) {
	// 递归打印树结构
	var printTree func(node *types.TreeNode, indent string)
	printTree = func(node *types.TreeNode, indent string) {
		log.Printf("[DEBUG] %s├── %s (%s) - 子节点数: %d", indent, node.Name, node.Type, len(node.Children))
		for i := range node.Children {
			newIndent := indent + "│  "
			if i == len(node.Children)-1 {
				newIndent = indent + "   "
			}
			printTree(node.Children[i], newIndent)
		}
	}
	printTree(tree, "")
}

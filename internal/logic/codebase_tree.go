package logic

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

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
	if l.svcCtx.VectorStore == nil {
		return nil, fmt.Errorf("VectorStore 未初始化")
	}

	records, err := l.svcCtx.VectorStore.GetCodebaseRecords(l.ctx, codebaseId, codebasePath)
	if err != nil {
		return nil, fmt.Errorf("查询文件路径失败: %w", err)
	}

	if len(records) == 0 {
		l.logEmptyRecordsDiagnostic(codebaseId, codebasePath)
	}

	// 合并相同文件路径的记录
	log.Printf("[DEBUG] 开始合并相同文件路径的记录...")
	mergedRecords, mergeCount := l.mergeRecordsByFilePath(records)
	log.Printf("[DEBUG] 合并完成：原始记录数=%d，合并后记录数=%d，合并了%d个重复路径",
		len(records), len(mergedRecords), mergeCount)

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

	for _, fileRecords := range filePathMap {
		if len(fileRecords) == 1 {
			// 没有重复，直接添加
			mergedRecords = append(mergedRecords, fileRecords[0])
		} else {
			// 有重复，合并记录
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
	log.Printf("[DEBUG] 🔍 关键诊断：多级路径处理分析开始")
	normalizedPaths := make([]string, len(filePaths))
	for i, path := range filePaths {
		normalizedPaths[i] = normalizePath(path)
	}
	filePaths = normalizedPaths
	log.Printf("[DEBUG] 🔍 关键诊断：多级路径规范化处理完成")

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

		if !includeFiles && !isDirectory(filePath) {
			skippedFiles++
			continue
		}

		// 🔧 修复：改进的相对路径计算逻辑，支持多级路径
		var relativePath string
		if rootPath == "." {
			// 当根路径为 "." 时，不应该去掉任何字符
			relativePath = filePath
		} else {
			// 🔧 修复：确保根路径匹配后再进行截取
			if strings.HasPrefix(filePath, rootPath) {
				// 原有逻辑：去掉根路径部分
				relativePath = filePath[len(rootPath):]
				log.Printf("[DEBUG] ✅ 使用原有逻辑计算相对路径")
			} else {
				// 🔧 修复：如果文件路径不以根路径开头，可能是路径规范化问题
				// 尝试使用规范化后的路径进行比较
				normalizedFilePath := normalizePath(filePath)
				normalizedRootPath := normalizePath(rootPath)

				if strings.HasPrefix(normalizedFilePath, normalizedRootPath) {
					relativePath = normalizedFilePath[len(normalizedRootPath):]
				} else {
					relativePath = filePath
				}
			}
		}

		// 🔧 修复：更安全地移除开头的分隔符
		if len(relativePath) > 0 {
			firstChar := relativePath[0]
			if firstChar == '/' || firstChar == '\\' {
				relativePath = relativePath[1:]
			}
		}

		currentDepth := strings.Count(relativePath, string(filepath.Separator))
		log.Printf("[DEBUG] 深度计算 - FilePath: '%s', RootPath: '%s', RelativePath: '%s', Depth: %d",
			filePath, rootPath, relativePath, currentDepth)

		if maxDepth > 0 && currentDepth > maxDepth {
			skippedFiles++
			continue
		}

		// 🔧 修复：确保所有路径都使用规范化格式
		// 构建路径节点
		dir := normalizePath(filepath.Dir(filePath))

		// 添加诊断日志：显示文件路径分析
		log.Printf("[DEBUG] ===== 数据流跟踪：文件路径处理 =====")

		{
			// 路径组件分析
			pathComponents := strings.Split(filePath, string(filepath.Separator))

			// 给文件创建目录
			mountPath := ""
			currentPath := ""
			for idx, pathComponent := range pathComponents {
				if idx+1 == len(pathComponents) {
					break
				}

				if currentPath == "" {
					currentPath = pathComponent
				} else {
					currentPath = currentPath + "\\" + pathComponent
					currentPath = normalizePath(currentPath)
				}

				// 存在当前路径，则跳过，不创建
				if _, exists := pathMap[currentPath]; exists {
					if mountPath == "" {
						mountPath = pathComponent
					} else {
						mountPath = mountPath + "\\" + pathComponent
						mountPath = normalizePath(mountPath)
					}
					continue
				}

				// 创建目录
				node := &types.TreeNode{
					Name:     filepath.Base(pathComponent),
					Path:     currentPath,
					Type:     "directory",
					Children: make([]*types.TreeNode, 0),
				}
				pathMap[currentPath] = node

				// 挂载目录
				if _, exists := pathMap[mountPath]; exists {
					pathMap[mountPath].Children = append(pathMap[mountPath].Children, node)
				} else {
					pathMap[rootPath].Children = append(pathMap[rootPath].Children, node)
				}
				if mountPath == "" {
					mountPath = pathComponent
				} else {
					mountPath = mountPath + "\\" + pathComponent
					mountPath = normalizePath(mountPath)
				}
			}

		}

		// 添加文件节点
		if includeFiles && !isDirectory(filePath) {
			processedFilesCount++

			fileNode, err := createFileNode(filePath)
			if err != nil {
				continue
			}

			parentFound := false
			var foundParentNode *types.TreeNode
			normalizedDir := normalizePath(dir)

			for path, parentNode := range pathMap {
				if path == normalizedDir {
					foundParentNode = parentNode
					parentFound = true
					break
				}
			}
			if parentFound && foundParentNode != nil {
				foundParentNode.Children = append(foundParentNode.Children, fileNode)
			} else {
				if dir == rootPath {
					root.Children = append(root.Children, fileNode)
				}
			}
		}
	}

	return root, nil
}

// extractRootPath 提取根路径
func extractRootPath(filePaths []string) string {
	if len(filePaths) == 0 {
		return ""
	}

	// 分析路径深度分布（使用规范化后的路径）
	depthAnalysis := make(map[int]int)
	for _, path := range filePaths {
		depth := strings.Count(path, string(filepath.Separator))
		depthAnalysis[depth]++
	}

	if len(filePaths) == 0 {
		return ""
	}

	// 首先分析所有路径的深度，确保找到正确的公共前缀
	minDepth := int(^uint(0) >> 1) // 最大int值
	for _, path := range filePaths {
		depth := strings.Count(path, string(filepath.Separator))
		if depth < minDepth {
			minDepth = depth
		}
	}

	// 使用改进的算法，考虑路径组件的匹配
	commonPrefix := filePaths[0]
	log.Printf("[DEBUG] 初始公共前缀（第一个路径）: '%s'", commonPrefix)

	for _, path := range filePaths[1:] {
		newPrefix := findCommonPrefix(commonPrefix, path)

		commonPrefix = newPrefix
		if commonPrefix == "" {
			break
		}
	}

	// 🔧 修复：如果公共前缀不以目录分隔符结尾，找到最后一个分隔符
	lastSeparator := strings.LastIndexAny(commonPrefix, string(filepath.Separator))

	if lastSeparator == -1 {
		// 🔧 修复：对于多级路径，如果没有共同的目录前缀，尝试找到父目录
		// 检查是否所有路径都有相同的第一级目录
		firstComponents := make([]string, len(filePaths))
		allHaveSameFirstComponent := true
		var firstComponent string

		for i, path := range filePaths {
			components := strings.Split(path, string(filepath.Separator))
			if len(components) > 0 {
				if i == 0 {
					firstComponent = components[0]
				} else if components[0] != firstComponent {
					allHaveSameFirstComponent = false
					break
				}
				firstComponents[i] = components[0]
			}
		}

		if allHaveSameFirstComponent && firstComponent != "" {
			return firstComponent
		} else {
			return "."
		}
	}

	rootPath := commonPrefix[:lastSeparator+1]

	// 🔧 修复：确保根路径也被规范化
	rootPath = normalizePath(rootPath)

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
	if path == "" {
		return ""
	}

	// 🔧 修复：处理多级路径的特殊情况
	// 首先统一使用 / 作为分隔符进行处理
	unifiedPath := strings.ReplaceAll(path, "\\", "/")

	// 使用 filepath.Clean 进行基本规范化
	cleaned := filepath.Clean(unifiedPath)

	// 再次确保路径使用系统标准的分隔符
	// 在 Windows 上，这会将 / 转换为 \
	// 在 Unix 上，这会将 \ 转换为 /
	normalized := filepath.FromSlash(cleaned)

	// 🔧 修复：确保多级路径的格式一致性
	// 如果路径以分隔符结尾，移除它（除非是根目录）
	if len(normalized) > 1 && (strings.HasSuffix(normalized, "\\") || strings.HasSuffix(normalized, "/")) {
		normalized = normalized[:len(normalized)-1]
	}

	return normalized
}

// createFileNode 创建文件节点
func createFileNode(filePath string) (*types.TreeNode, error) {
	normalizedPath := normalizePath(filePath)

	node := &types.TreeNode{
		Name: filepath.Base(normalizedPath),
		Path: normalizedPath, // 🔧 修复：使用规范化后的路径
		Type: "file",
	}
	return node, nil
}

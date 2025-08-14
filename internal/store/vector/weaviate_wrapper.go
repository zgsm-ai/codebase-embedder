package vector

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/store/redis"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"

	"github.com/weaviate/weaviate/entities/vectorindex/dynamic"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	goweaviate "github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/auth"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"
	"github.com/zgsm-ai/codebase-indexer/internal/config"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

type weaviateWrapper struct {
	reranker      Reranker
	embedder      Embedder
	client        *goweaviate.Client
	className     string
	cfg           config.VectorStoreConf
	statusManager *redis.StatusManager
	requestId     string
}

func New(cfg config.VectorStoreConf, embedder Embedder, reranker Reranker) (Store, error) {
	var authConf auth.Config
	if cfg.Weaviate.APIKey != types.EmptyString {
		authConf = auth.ApiKey{Value: cfg.Weaviate.APIKey}
	}
	client, err := goweaviate.NewClient(goweaviate.Config{
		Host:       cfg.Weaviate.Endpoint,
		Scheme:     schemeHttp,
		AuthConfig: authConf,
		Timeout:    cfg.Weaviate.Timeout,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create Weaviate client: %w", err)
	}

	store := &weaviateWrapper{
		client:    client,
		className: cfg.Weaviate.ClassName,
		embedder:  embedder,
		reranker:  reranker,
		cfg:       cfg,
	}

	// init class
	err = store.createClassWithAutoTenantEnabled(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create class: %w", err)
	}

	return store, nil
}

// NewWithStatusManager creates a new instance of weaviateWrapper with status manager
func NewWithStatusManager(cfg config.VectorStoreConf, embedder Embedder, reranker Reranker, statusManager *redis.StatusManager, requestId string) (Store, error) {
	var authConf auth.Config
	if cfg.Weaviate.APIKey != types.EmptyString {
		authConf = auth.ApiKey{Value: cfg.Weaviate.APIKey}
	}
	client, err := goweaviate.NewClient(goweaviate.Config{
		Host:       cfg.Weaviate.Endpoint,
		Scheme:     schemeHttp,
		AuthConfig: authConf,
		Timeout:    cfg.Weaviate.Timeout,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create Weaviate client: %w", err)
	}

	store := &weaviateWrapper{
		client:        client,
		className:     cfg.Weaviate.ClassName,
		embedder:      embedder,
		reranker:      reranker,
		cfg:           cfg,
		statusManager: statusManager,
		requestId:     requestId,
	}

	// init class
	err = store.createClassWithAutoTenantEnabled(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create class: %w", err)
	}

	return store, nil
}

func (r *weaviateWrapper) GetIndexSummary(ctx context.Context, codebaseId int32, codebasePath string) (*types.EmbeddingSummary, error) {
	start := time.Now()
	tenantName, err := r.generateTenantName(codebasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tenant name: %w", err)
	}

	// Define GraphQL fields using proper Field type
	fields := []graphql.Field{
		{Name: "meta", Fields: []graphql.Field{
			{Name: "count"},
		}},
		{Name: "groupedBy", Fields: []graphql.Field{
			{Name: "path"},
			{Name: "value"},
		}},
	}

	codebaseFilter := filters.Where().WithPath([]string{MetadataCodebaseId}).
		WithOperator(filters.Equal).WithValueInt(int64(codebaseId))

	res, err := r.client.GraphQL().Aggregate().
		WithClassName(r.className).
		WithFields(fields...).
		WithWhere(codebaseFilter).
		WithGroupBy(MetadataFilePath).
		WithTenant(tenantName).
		Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get index summary: %w", err)
	}

	summary, err := r.unmarshalSummarySearchResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal summary response: %w", err)
	}
	tracer.WithTrace(ctx).Infof("embedding getIndexSummary end, cost %d ms on total %d files %d chunks",
		time.Since(start).Milliseconds(), summary.TotalFiles, summary.TotalChunks)
	return summary, nil
}

func (r *weaviateWrapper) DeleteCodeChunks(ctx context.Context, chunks []*types.CodeChunk, options Options) error {
	if len(chunks) == 0 {
		return nil // Nothing to delete
	}

	tenant, err := r.generateTenantName(options.CodebasePath)
	if err != nil {
		return err
	}
	// Build a list of filters, one for each codebaseId and filePath pair
	chunkFilters := make([]*filters.WhereBuilder, len(chunks))
	for i, chunk := range chunks {
		if chunk.CodebaseId == 0 || chunk.FilePath == types.EmptyString {
			return fmt.Errorf("invalid chunk to delete: required codebaseId and filePath")
		}
		chunkFilters[i] = filters.Where().
			WithOperator(filters.And).
			WithOperands([]*filters.WhereBuilder{
				filters.Where().
					WithPath([]string{MetadataCodebaseId}).
					WithOperator(filters.Equal).
					WithValueInt(int64(chunk.CodebaseId)),
				filters.Where().
					WithPath([]string{MetadataFilePath}).
					WithOperator(filters.Equal).
					WithValueText(chunk.FilePath),
			})
	}

	// Combine all chunk filters with OR to support batch deletion of files
	combinedFilter := filters.Where().
		WithOperator(filters.Or).
		WithOperands(chunkFilters)

	do, err := r.client.Batch().ObjectsBatchDeleter().
		WithTenant(tenant).WithWhere(
		combinedFilter,
	).WithClassName(r.className).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to send delete chunks err:%w", err)
	}
	return CheckBatchDeleteErrors(do)
}

func (r *weaviateWrapper) SimilaritySearch(ctx context.Context, query string, numDocuments int, options Options) ([]*types.SemanticFileItem, error) {
	embedQuery, err := r.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	tenantName, err := r.generateTenantName(options.CodebasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tenant name: %w", err)
	}

	// Define GraphQL fields using proper Field type
	fields := []graphql.Field{
		{Name: MetadataCodebaseId},
		{Name: MetadataCodebaseName},
		{Name: MetadataSyncId},
		{Name: MetadataCodebasePath},
		{Name: MetadataFilePath},
		{Name: MetadataLanguage},
		{Name: MetadataRange},
		{Name: MetadataTokenCount},
		{Name: Content},
		{Name: "_additional", Fields: []graphql.Field{
			{Name: "certainty"},
			{Name: "distance"},
			{Name: "id"},
		}},
	}

	// Build GraphQL query with proper tenant filter
	nearVector := r.client.GraphQL().NearVectorArgBuilder().
		WithVector(embedQuery)

	res, err := r.client.GraphQL().Get().
		WithClassName(r.className).
		WithFields(fields...).
		WithNearVector(nearVector).
		WithLimit(numDocuments).
		WithTenant(tenantName).
		Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to execute similarity search: %w", err)
	}

	// Improved error handling for response validation
	if res == nil || res.Data == nil {
		return nil, fmt.Errorf("received empty response from Weaviate")
	}
	if err = CheckGraphQLResponseError(res); err != nil {
		return nil, fmt.Errorf("query weaviate failed: %w", err)
	}

	items, err := r.unmarshalSimilarSearchResponse(res, options.CodebasePath, options.ClientId, options.Authorization)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return items, nil
}

func (r *weaviateWrapper) unmarshalSimilarSearchResponse(res *models.GraphQLResponse, codebasePath, clientId string, authorization string) ([]*types.SemanticFileItem, error) {
	// Get the data for our class
	data, ok := res.Data["Get"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: 'Get' field not found or has wrong type")
	}

	results, ok := data[r.className].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: class data not found or has wrong type")
	}

	items := make([]*types.SemanticFileItem, 0, len(results))
	for _, result := range results {
		obj, ok := result.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract additional properties
		additional, ok := obj["_additional"].(map[string]interface{})
		if !ok {
			continue
		}

		content := getStringValue(obj, Content)
		filePath := getStringValue(obj, MetadataFilePath)

		// 如果开启获取源码，则从MetadataRange中提取行号信息
		if r.cfg.FetchSourceCode && content != "" && filePath != "" {
			// 从MetadataRange中提取startLine和endLine
			var startLine, endLine int
			if rangeValue, ok := obj[MetadataRange].([]interface{}); ok && len(rangeValue) >= 2 {
				if first, ok := rangeValue[0].(float64); ok {
					startLine = int(first)
				}
				if second, ok := rangeValue[2].(float64); ok {
					endLine = int(second)
				}
			}

			// 通过fetchCodeContent接口获取代码片段
			if codebasePath != "" {
				fetchedContent, err := fetchCodeContent(context.Background(), r.cfg, clientId, codebasePath, filePath, startLine, endLine, authorization)
				if err == nil && fetchedContent != "" {
					content = fetchedContent
				}
			}
		}

		fmt.Printf("[DEBUG] %s content: %s\n", filePath, content)

		// Create SemanticFileItem with proper fields
		item := &types.SemanticFileItem{
			Content:  content,
			FilePath: filePath,
			Score:    float32(getFloatValue(additional, "certainty")), // Convert float64 to float32
		}

		items = append(items, item)
	}

	return items, nil
}

// Helper functions for safe type conversion
func getStringValue(obj map[string]interface{}, key string) string {
	if val, ok := obj[key].(string); ok {
		return val
	}
	return ""
}

func getFloatValue(obj map[string]interface{}, key string) float64 {
	if val, ok := obj[key].(float64); ok {
		return val
	}
	return 0
}

func (r *weaviateWrapper) GetCodebaseRecords(ctx context.Context, codebaseId int32, codebasePath string) ([]*types.CodebaseRecord, error) {
	// 添加调试日志
	fmt.Printf("[DEBUG] GetCodebaseRecords - 开始执行，codebaseId: %d, codebasePath: %s\n", codebaseId, codebasePath)

	// 检查输入参数
	if codebaseId == 0 {
		fmt.Printf("[DEBUG] 警告: codebaseId 为 0，这可能不正确\n")
	}
	if codebasePath == "" {
		fmt.Printf("[DEBUG] 警告: codebasePath 为空字符串\n")
	}

	tenantName, err := r.generateTenantName(codebasePath)
	if err != nil {
		fmt.Printf("[DEBUG] 生成 tenantName 失败: %v\n", err)
		return nil, fmt.Errorf("failed to generate tenant name: %w", err)
	}
	fmt.Printf("[DEBUG] 生成的 tenantName: %s\n", tenantName)

	// 添加调试日志：检查 Weaviate 连接状态
	fmt.Printf("[DEBUG] 检查 Weaviate 连接状态...\n")
	live, err := r.client.Misc().LiveChecker().Do(ctx)
	if err != nil {
		fmt.Printf("[DEBUG] Weaviate 连接检查失败: %v\n", err)
	} else {
		fmt.Printf("[DEBUG] Weaviate 连接状态: %v\n", live)
	}

	// 定义GraphQL字段
	fields := []graphql.Field{
		{Name: "_additional", Fields: []graphql.Field{
			{Name: "id"},
			{Name: "lastUpdateTimeUnix"},
		}},
		{Name: MetadataFilePath},
		{Name: MetadataLanguage},
		{Name: Content},
		{Name: MetadataRange},
		{Name: MetadataTokenCount},
	}

	// 构建过滤器
	codebaseFilter := filters.Where().WithPath([]string{MetadataCodebaseId}).
		WithOperator(filters.Equal).WithValueInt(int64(codebaseId))

	fmt.Printf("[DEBUG] 构建的过滤器 - codebaseId: %d\n", codebaseId)
	fmt.Printf("[DEBUG] 查询类名: %s\n", r.className)
	fmt.Printf("[DEBUG] 使用的 tenant: %s\n", tenantName)

	// 执行查询，获取所有记录
	var allRecords []*types.CodebaseRecord
	limit := 1000 // 每批获取1000条记录
	offset := 0

	for {
		fmt.Printf("[DEBUG] 执行 GraphQL 查询 - offset: %d, limit: %d\n", offset, limit)
		fmt.Printf("[DEBUG] GraphQL 查询参数 - className: %s, tenant: %s, codebaseId: %d\n",
			r.className, tenantName, codebaseId)

		res, err := r.client.GraphQL().Get().
			WithClassName(r.className).
			WithFields(fields...).
			WithWhere(codebaseFilter).
			WithLimit(limit).
			WithOffset(offset).
			WithTenant(tenantName).
			Do(ctx)

		if err != nil {
			fmt.Printf("[DEBUG] GraphQL 查询失败: %v\n", err)
			fmt.Printf("[DEBUG] 错误详情 - 可能是 tenant 不存在或权限问题\n")
			return nil, fmt.Errorf("failed to get codebase records: %w", err)
		}

		if res == nil || res.Data == nil {
			fmt.Printf("[DEBUG] 响应为空，结束查询 - 可能 tenant %s 中没有数据\n", tenantName)
			break
		}

		// 解析响应
		records, err := r.unmarshalCodebaseRecordsResponse(res)
		if err != nil {
			fmt.Printf("[DEBUG] 解析响应失败: %v\n", err)
			return nil, fmt.Errorf("failed to unmarshal records response: %w", err)
		}

		fmt.Printf("[DEBUG] 本批次获取记录数: %d\n", len(records))
		if len(records) == 0 {
			fmt.Printf("[DEBUG] 没有更多记录，结束查询 - tenant %s 中可能没有 codebaseId %d 的数据\n", tenantName, codebaseId)
			break
		}

		allRecords = append(allRecords, records...)
		offset += limit

		// 如果获取的记录数小于limit，说明已经获取完所有记录
		if len(records) < limit {
			break
		}
	}

	fmt.Printf("[DEBUG] 查询完成，总记录数: %d\n", len(allRecords))

	// 分析查询结果
	if len(allRecords) == 0 {
		fmt.Printf("[DEBUG] 警告: 查询结果为空，可能的原因:\n")
		fmt.Printf("[DEBUG] 1. Weaviate 中没有 codebaseId %d 的数据\n", codebaseId)
		fmt.Printf("[DEBUG] 2. Tenant %s 不存在或没有权限访问\n", tenantName)
		fmt.Printf("[DEBUG] 3. 过滤器条件过于严格\n")
		fmt.Printf("[DEBUG] 4. Weaviate 连接配置有问题\n")
		fmt.Printf("[DEBUG] 5. 类名 %s 不正确\n", r.className)
	} else {
		// 分析文件路径分布
		pathAnalysis := make(map[string]int)
		for _, record := range allRecords {
			pathAnalysis[record.FilePath]++
		}
		fmt.Printf("[DEBUG] 查询结果分析:\n")
		fmt.Printf("[DEBUG]   唯一文件路径数: %d\n", len(pathAnalysis))
		fmt.Printf("[DEBUG]   总记录数: %d\n", len(allRecords))

		// 显示前几个文件路径作为示例
		count := 0
		for path := range pathAnalysis {
			if count < 5 {
				fmt.Printf("[DEBUG]   示例文件路径 %d: %s\n", count+1, path)
				count++
			} else {
				break
			}
		}
		if len(pathAnalysis) > 5 {
			fmt.Printf("[DEBUG]   ... (还有 %d 个文件路径未显示)\n", len(pathAnalysis)-5)
		}
	}

	return allRecords, nil
}

func (r *weaviateWrapper) unmarshalCodebaseRecordsResponse(res *models.GraphQLResponse) ([]*types.CodebaseRecord, error) {
	if len(res.Errors) > 0 {
		var errMsg string
		for _, e := range res.Errors {
			errMsg += e.Message
		}
		return nil, fmt.Errorf("failed to get codebase records: %s", errMsg)
	}

	// 检查响应是否为空
	if res == nil || res.Data == nil {
		fmt.Printf("[DEBUG] 响应为空，返回 nil 记录\n")
		return nil, nil
	}

	// 获取 Get 字段
	data, ok := res.Data["Get"].(map[string]interface{})
	if !ok {
		fmt.Printf("[DEBUG] 响应格式错误：'Get' 字段不存在或类型错误\n")
		return nil, fmt.Errorf("invalid response format: 'Get' field not found or has wrong type")
	}

	// 获取类名对应的数据
	results, ok := data[r.className].([]interface{})
	if !ok {
		fmt.Printf("[DEBUG] 响应格式错误：类数据不存在或类型错误，类名: %s\n", r.className)
		return nil, fmt.Errorf("invalid response format: class data not found or has wrong type")
	}

	fmt.Printf("[DEBUG] 解析响应，原始结果数量: %d\n", len(results))

	records := make([]*types.CodebaseRecord, 0, len(results))
	uniquePaths := make(map[string]int) // 跟踪唯一路径

	for i, result := range results {
		obj, ok := result.(map[string]interface{})
		if !ok {
			fmt.Printf("[DEBUG] 跳过结果 %d：不是有效的 map[string]interface{} 类型\n", i)
			continue
		}

		// 提取附加属性
		additional, ok := obj["_additional"].(map[string]interface{})
		if !ok {
			fmt.Printf("[DEBUG] 跳过结果 %d：_additional 字段不存在或类型错误\n", i)
			continue
		}

		// 解析最后更新时间
		var lastUpdated time.Time
		if lastUpdateUnix, ok := additional["lastUpdateTimeUnix"].(float64); ok {
			lastUpdated = time.Unix(int64(lastUpdateUnix), 0)
		} else {
			lastUpdated = time.Now()
		}

		// 解析范围信息
		var rangeInfo []int
		if rangeData, ok := obj[MetadataRange].([]interface{}); ok {
			rangeInfo = make([]int, len(rangeData))
			for i, v := range rangeData {
				if num, ok := v.(float64); ok {
					rangeInfo[i] = int(num)
				}
			}
		}

		filePath := getStringValue(obj, MetadataFilePath)
		record := &types.CodebaseRecord{
			Id:          getStringValue(additional, "id"),
			FilePath:    filePath,
			Language:    getStringValue(obj, MetadataLanguage),
			Content:     getStringValue(obj, Content),
			Range:       rangeInfo,
			TokenCount:  int(getFloatValue(obj, MetadataTokenCount)),
			LastUpdated: lastUpdated,
		}

		records = append(records, record)
		uniquePaths[filePath]++

		if i < 10 { // 只打印前10个记录避免日志过多
			fmt.Printf("[DEBUG] 记录 %d: FilePath=%s, Language=%s, ID=%s\n", i+1, filePath, record.Language, record.Id)
		}
	}

	fmt.Printf("[DEBUG] 解析完成，有效记录数: %d\n", len(records))
	fmt.Printf("[DEBUG] 唯一文件路径数: %d\n", len(uniquePaths))
	for path, count := range uniquePaths {
		if count > 1 {
			fmt.Printf("[DEBUG] 重复路径: %s (出现 %d 次)\n", path, count)
		}
	}

	return records, nil
}

func (r *weaviateWrapper) Close() {
}

func (r *weaviateWrapper) DeleteByCodebase(ctx context.Context, codebaseId int32, codebasePath string) error {

	tenant, err := r.generateTenantName(codebasePath)
	if err != nil {
		return err
	}
	codebaseFilter := filters.Where().WithPath([]string{MetadataCodebaseId}).
		WithOperator(filters.Equal).WithValueInt(int64(codebaseId))

	do, err := r.client.Batch().ObjectsBatchDeleter().
		WithTenant(tenant).WithWhere(
		codebaseFilter,
	).WithClassName(r.className).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to send delete codebase chunks, err:%w", err)
	}
	return CheckBatchDeleteErrors(do)
}

func (r *weaviateWrapper) UpsertCodeChunks(ctx context.Context, docs []*types.CodeChunk, options Options) error {
	if len(docs) == 0 {
		return nil
	}
	// TODO 事务保障
	// 先删除已有的相同codebaseId和FilePath的数据，避免重复
	//TODO 启动一个定时任务，清理重复数据。根据CodebaseId、FilePaths、Content 去重。
	// TODO 区分添加、修改、删除场景， 只有修改/删除需要先delete，添加不用。
	err := r.DeleteCodeChunks(ctx, docs, options)
	if err != nil {
		tracer.WithTrace(ctx).Errorf("[%s]failed to delete existing code chunks before upsert: %v", docs[0].CodebasePath, err)
	}

	return r.InsertCodeChunks(ctx, docs, options)
}

func (r *weaviateWrapper) InsertCodeChunks(ctx context.Context, docs []*types.CodeChunk, options Options) error {
	if len(docs) == 0 {
		return nil
	}
	tenantName, err := r.generateTenantName(docs[0].CodebasePath)
	if err != nil {
		return err
	}

	tracer.WithTrace(ctx).Infof("InsertCodeChunks options.RequestId: %s ", options.RequestId)
	// 如果有状态管理器和请求ID，则使用带有状态管理器的 embedder
	var chunks []*CodeChunkEmbedding
	if r.statusManager != nil && options.RequestId != "" {
		// 创建带有状态管理器的临时 embedder
		embedderWithStatus, err := NewEmbedderWithStatusManager(r.cfg.Embedder, r.statusManager, options.RequestId, options.TotalFiles)
		if err != nil {
			return fmt.Errorf("failed to create embedder with status manager: %w", err)
		}
		chunks, err = embedderWithStatus.EmbedCodeChunks(ctx, docs)
	} else {
		// 使用原有的 embedder
		chunks, err = r.embedder.EmbedCodeChunks(ctx, docs)
	}

	if err != nil {
		return err
	}
	tracer.WithTrace(ctx).Infof("embedded %d chunks for codebase %s successfully", len(docs), docs[0].CodebaseName)

	objs := make([]*models.Object, len(chunks), len(chunks))
	for i, c := range chunks {
		if c.FilePath == types.EmptyString || c.CodebaseId == 0 || c.CodebasePath == types.EmptyString {
			return fmt.Errorf("invalid chunk to write: required fields: CodebaseId, CodebasePath, FilePaths")
		}
		objs[i] = &models.Object{
			ID:     strfmt.UUID(uuid.New().String()),
			Class:  r.className,
			Tenant: tenantName,
			Vector: c.Embedding,
			Properties: map[string]any{
				MetadataFilePath:     c.FilePath,
				MetadataLanguage:     c.Language,
				MetadataCodebaseId:   c.CodebaseId,
				MetadataCodebasePath: c.CodebasePath,
				MetadataCodebaseName: c.CodebaseName,
				MetadataSyncId:       options.SyncId,
				MetadataRange:        c.Range,
				MetadataTokenCount:   c.TokenCount,
				Content:              string(c.Content),
			},
		}
	}
	tracer.WithTrace(ctx).Infof("start to save %d chunks for codebase %s successfully", len(docs), docs[0].CodebaseName)
	resp, err := r.client.Batch().ObjectsBatcher().WithObjects(objs...).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to send batch to Weaviate: %w", err)
	}
	if err = CheckBatchErrors(resp); err != nil {
		return fmt.Errorf("failed to send batch to Weaviate: %w", err)
	}
	tracer.WithTrace(ctx).Infof("save %d chunks for codebase %s successfully", len(docs), docs[0].CodebaseName)
	return nil
}

func (r *weaviateWrapper) Query(ctx context.Context, query string, topK int, options Options) ([]*types.SemanticFileItem, error) {
	documents, err := r.SimilaritySearch(ctx, query, r.cfg.Weaviate.MaxDocuments, options)

	if err != nil {
		return nil, err
	}
	//  调用reranker模型进行重排
	rerankedDocs, err := r.reranker.Rerank(ctx, query, documents)
	if err != nil {
		tracer.WithTrace(ctx).Errorf("failed customReranker docs: %v", err)
	}
	if len(rerankedDocs) == 0 {
		rerankedDocs = documents
	}
	// topK
	rerankedDocs = rerankedDocs[:int(math.Min(float64(topK), float64(len(rerankedDocs))))]
	return rerankedDocs, nil
}

// fetchCodeContent 通过API获取代码片段的Content
func fetchCodeContent(ctx context.Context, cfg config.VectorStoreConf, clientId, codebasePath, filePath string, startLine, endLine int, authorization string) (string, error) {
	// 构建API请求URL
	baseURL := cfg.BaseURL

	// 对参数进行URL编码
	encodedCodebasePath := url.QueryEscape(codebasePath)

	// 如果filePath是全路径，则与codebasePath拼接处理
	var processedFilePath string
	if strings.HasPrefix(filePath, "/") {
		// filePath是全路径，直接使用
		processedFilePath = filePath
	} else {
		// filePath是相对路径，与codebasePath拼接
		processedFilePath = fmt.Sprintf("%s/%s", strings.TrimSuffix(codebasePath, "/"), filePath)
	}

	// 检查操作系统类型，如果是Windows则将路径转换为Windows格式
	if runtime.GOOS == "windows" {
		// 将Unix风格的路径转换为Windows风格
		processedFilePath = filepath.FromSlash(processedFilePath)
		// 确保路径是绝对路径格式
		if !strings.HasPrefix(processedFilePath, "\\") && !strings.Contains(processedFilePath, ":") {
			// 如果不是网络路径也不是驱动器路径，添加当前驱动器
			processedFilePath = filepath.Join(filepath.VolumeName("."), processedFilePath)
		}
	}

	encodedFilePath := url.QueryEscape(processedFilePath)

	// 构建完整的请求URL
	requestURL := fmt.Sprintf("%s?clientId=%s&codebasePath=%s&filePath=%s&startLine=%d&endLine=%d",
		baseURL, clientId, encodedCodebasePath, encodedFilePath, startLine, endLine)

	tracer.WithTrace(ctx).Infof("fetchCodeContent %s: ", requestURL)

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 添加Authorization头
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	// 发送HTTP GET请求
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch code content: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

func (r *weaviateWrapper) createClassWithAutoTenantEnabled(client *goweaviate.Client) error {
	timeout, cancelFunc := context.WithTimeout(context.Background(), time.Second*60)
	defer cancelFunc()
	tracer.WithTrace(timeout).Infof("start to create weaviate class %s", r.className)
	res, err := client.Schema().ClassExistenceChecker().WithClassName(r.className).Do(timeout)
	if err != nil {
		tracer.WithTrace(timeout).Errorf("check weaviate class exists err:%v", err)
	}
	if err == nil && res {
		tracer.WithTrace(timeout).Infof("weaviate class %s already exists, not create.", r.className)
		return nil
	}

	// 定义类的属性并配置索引
	dynamicConf := dynamic.NewDefaultUserConfig()
	class := &models.Class{
		Class:      r.className,
		Properties: classProperties, // fields
		// auto create tenant
		MultiTenancyConfig: &models.MultiTenancyConfig{
			Enabled:            true,
			AutoTenantCreation: true,
		},
		VectorIndexType:   dynamicConf.IndexType(),
		VectorIndexConfig: dynamicConf,
	}

	tracer.WithTrace(timeout).Infof("class info:%v", class)
	err = client.Schema().ClassCreator().WithClass(class).Do(timeout)
	// TODO skip already exists err
	if err != nil && strings.Contains(err.Error(), "already exists") {
		tracer.WithTrace(timeout).Infof("weaviate class %s already exists, not create.", r.className)
		return nil
	}
	tracer.WithTrace(timeout).Infof("weaviate class %s end.", r.className)
	return err
}

// generateTenantName 使用 MD5 哈希生成合规租户名（32字符，纯十六进制）
func (r *weaviateWrapper) generateTenantName(codebasePath string) (string, error) {
	// 添加调试日志
	fmt.Printf("[DEBUG] generateTenantName - 输入 codebasePath: %s\n", codebasePath)

	if codebasePath == types.EmptyString {
		fmt.Printf("[DEBUG] generateTenantName - codebasePath 为空字符串\n")
		return types.EmptyString, ErrInvalidCodebasePath
	}
	hash := md5.Sum([]byte(codebasePath))     // 计算 MD5 哈希
	tenantName := hex.EncodeToString(hash[:]) // 转为32位十六进制字符串

	fmt.Printf("[DEBUG] generateTenantName - 生成的 tenantName: %s\n", tenantName)
	return tenantName, nil
}

func (r *weaviateWrapper) unmarshalSummarySearchResponse(res *models.GraphQLResponse) (*types.EmbeddingSummary, error) {
	if len(res.Errors) > 0 {
		var errMsg string
		for _, e := range res.Errors {
			errMsg += e.Message
		}
		return nil, fmt.Errorf("failed to get embedding summary: %s", errMsg)
	}
	// 检查响应是否为空
	if res == nil || res.Data == nil {
		return nil, fmt.Errorf("received empty response from Weaviate")
	}

	// 获取 Aggregate 字段
	data, ok := res.Data["Aggregate"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: 'Aggregate' field not found or has wrong type")
	}

	// 获取类名对应的数据
	results, ok := data[r.className].([]interface{})
	if !ok || len(results) == 0 {
		return nil, fmt.Errorf("invalid response format: class data not found or has wrong type：%s", reflect.TypeOf(results).String())
	}
	var totalChunks, totalFiles int
	for _, v := range results {
		// 获取 meta 字段
		result, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invaid response format, result has wrong type: %s", reflect.TypeOf(result).String())
		}
		meta, ok := result["meta"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid response format: 'meta' field not found or has wrong type:%s", reflect.TypeOf(meta).String())
		}

		// 获取总数
		count, ok := meta["count"].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid response format: 'count' field not found or has wrong type:%s", reflect.TypeOf(count).String())
		}
		totalChunks += int(count)
		totalFiles++

	}

	return &types.EmbeddingSummary{
		TotalFiles:  totalFiles,
		TotalChunks: totalChunks,
	}, nil
}

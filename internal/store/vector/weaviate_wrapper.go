package vector

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

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
	reranker  Reranker
	embedder  Embedder
	client    *goweaviate.Client
	className string
	cfg       config.VectorStoreConf
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

	items, err := r.unmarshalSimilarSearchResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return items, nil
}

func (r *weaviateWrapper) unmarshalSimilarSearchResponse(res *models.GraphQLResponse) ([]*types.SemanticFileItem, error) {
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

		// Create SemanticFileItem with proper fields
		item := &types.SemanticFileItem{
			Content:  getStringValue(obj, Content),
			FilePath: getStringValue(obj, MetadataFilePath),
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
	chunks, err := r.embedder.EmbedCodeChunks(ctx, docs)
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
	if codebasePath == types.EmptyString {
		return types.EmptyString, ErrInvalidCodebasePath
	}
	hash := md5.Sum([]byte(codebasePath))   // 计算 MD5 哈希
	return hex.EncodeToString(hash[:]), nil // 转为32位十六进制字符串
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

package vector

import (
	"context"
	"strings"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/config"
	"github.com/zgsm-ai/codebase-indexer/internal/store/redis"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// Embedder defines the interface for embedding operations
type Embedder interface {
	// EmbedCodeChunks creates embeddings for multiple code chunks
	EmbedCodeChunks(ctx context.Context, chunks []*types.CodeChunk) ([]*CodeChunkEmbedding, error)
	// EmbedQuery creates an embedding for a single query string
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// CodeChunkEmbedding represents a code chunk with its embedding vector
type CodeChunkEmbedding struct {
	*types.CodeChunk
	Embedding []float32
}

// customEmbedder implements the Embedder interface
type customEmbedder struct {
	config          config.EmbedderConf
	embeddingClient EmbeddingClient
	statusManager   *redis.StatusManager
	requestId       string
}

// NewEmbedder creates a new instance of Embedder
func NewEmbedder(cfg config.EmbedderConf) (Embedder, error) {
	embeddingClient := NewEmbeddingClient(cfg)

	return &customEmbedder{
		embeddingClient: embeddingClient,
		config:          cfg,
	}, nil
}

// NewEmbedderWithStatusManager creates a new instance of Embedder with status manager
func NewEmbedderWithStatusManager(cfg config.EmbedderConf, statusManager *redis.StatusManager, requestId string) (Embedder, error) {
	embeddingClient := NewEmbeddingClient(cfg)

	return &customEmbedder{
		embeddingClient: embeddingClient,
		config:          cfg,
		statusManager:   statusManager,
		requestId:       requestId,
	}, nil
}

// EmbedCodeChunks implements the Embedder interface
func (e *customEmbedder) EmbedCodeChunks(ctx context.Context, chunks []*types.CodeChunk) ([]*CodeChunkEmbedding, error) {
	if len(chunks) == 0 {
		return []*CodeChunkEmbedding{}, nil
	}

	embeds := make([]*CodeChunkEmbedding, 0, len(chunks))
	batchSize := e.config.BatchSize
	start := time.Now()
	tracer.WithTrace(ctx).Infof("start to embedding %d chunks for codebase:%s, batchSize: %d, requestId:%s", len(chunks), chunks[0].CodebasePath, batchSize, e.requestId)

	// 用于跟踪已处理的文件数量
	processedFiles := 0
	// 用于跟踪已处理的文件路径，避免重复计数
	processedFilePaths := make(map[string]bool)

	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		// 准备当前批次的内容
		batch := make([][]byte, end-start)
		for i := 0; i < end-start; i++ {
			batch[i] = chunks[start+i].Content
			tracer.WithTrace(ctx).Infof("execute to %s embedding %d", chunks[start+i].FilePath, len(batch[i]))
		}

		// 执行嵌入
		embeddings, err := e.doEmbeddings(ctx, batch)
		if err != nil {
			tracer.WithTrace(ctx).Errorf("e.doEmbeddings(ctx, batch) filed: %v ", err)
			// continue
			break
			// return nil, err
		}

		// 将嵌入结果与原始块关联
		for i, em := range embeddings {
			embeds = append(embeds, &CodeChunkEmbedding{
				CodeChunk: chunks[start+i],
				Embedding: em,
			})
			tracer.WithTrace(ctx).Errorf("embeddings progress: %v", chunks[start+i].FilePath)
			// 检查文件是否已经被处理过
			tracer.WithTrace(ctx).Errorf("%v   ===============================: %v", e.requestId, processedFilePaths)
			filePath := chunks[start+i].FilePath
			if !processedFilePaths[filePath] {
				processedFilePaths[filePath] = true
				processedFiles++

				tracer.WithTrace(ctx).Errorf("%v-----------------------------: %v", processedFiles, processedFilePaths[filePath])

				// 每处理10个文件就同步一次进度
				if processedFiles%10 == 0 && e.statusManager != nil && e.requestId != "" {
					progress := int(float64(processedFiles) / float64(len(processedFilePaths)) * 100)
					err := e.statusManager.UpdateFileStatus(ctx, e.requestId, func(status *types.FileStatusResponseData) {
						status.Process = "processing"
						status.TotalProgress = progress
					})
					if err != nil {
						tracer.WithTrace(ctx).Errorf("failed to update progress: %v", err)
					} else {
						tracer.WithTrace(ctx).Infof("updated progress: %d%% (%d/%d files)", progress, processedFiles, len(processedFilePaths))
					}
				}
			}
		}
	}

	// 最终更新一次进度
	if e.statusManager != nil && e.requestId != "" {
		progress := 100
		err := e.statusManager.UpdateFileStatus(ctx, e.requestId, func(status *types.FileStatusResponseData) {
			status.Process = "complete"
			status.TotalProgress = progress
		})
		if err != nil {
			tracer.WithTrace(ctx).Errorf("failed to update final progress: %v", err)
		} else {
			tracer.WithTrace(ctx).Infof("updated final progress: %d%% (%d/%d files)", progress, processedFiles, len(processedFilePaths))
		}
	}

	tracer.WithTrace(ctx).Infof("embedding %d chunks for codebase:%s successfully, cost %d ms", len(chunks),
		chunks[0].CodebasePath, time.Since(start).Milliseconds())

	return embeds, nil
}

// EmbedQuery implements the Embedder interface
func (e *customEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if e.config.StripNewLines {
		query = strings.ReplaceAll(query, "\n", " ")
	}
	tracer.WithTrace(ctx).Info("start to embed query")
	vectors, err := e.doEmbeddings(ctx, [][]byte{[]byte(query)})
	if err != nil {

		return nil, err
	}
	if len(vectors) == 0 {
		return nil, ErrEmptyResponse
	}
	tracer.WithTrace(ctx).Info("embed query successfully")
	return vectors[0], nil
}

// doEmbeddings performs the actual embedding operation
func (e *customEmbedder) doEmbeddings(ctx context.Context, textsByte [][]byte) ([][]float32, error) {
	texts := make([]string, len(textsByte))
	for i, b := range textsByte {
		texts[i] = string(b)
	}

	embeddings, err := e.embeddingClient.CreateEmbeddings(ctx, texts, e.config.Model)
	if err != nil {
		for _, text := range texts {
			tracer.WithTrace(ctx).Errorf("embed query file len %d failed, err: %v", len(text), err)
		}

		return nil, err
	}

	vectors := make([][]float32, len(textsByte))
	for i, embedding := range embeddings {
		transferredVector := make([]float32, 0, 768) // 768维
		for _, v := range embedding {
			transferredVector = append(transferredVector, float32(v))
		}
		vectors[i] = transferredVector
	}
	return vectors, nil
}

package vector

import (
	"context"
	"strings"
	"time"

	"github.com/zgsm-ai/codebase-indexer/internal/config"
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
}

// NewEmbedder creates a new instance of Embedder
func NewEmbedder(cfg config.EmbedderConf) (Embedder, error) {
	embeddingClient := NewEmbeddingClient(cfg)

	return &customEmbedder{
		embeddingClient: embeddingClient,
		config:          cfg,
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
	tracer.WithTrace(ctx).Infof("start to embedding %d chunks for codebase:%s, batchSize: %d ", len(chunks), chunks[0].CodebasePath, batchSize)

	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		// 准备当前批次的内容
		batch := make([][]byte, end-start)
		for i := 0; i < end-start; i++ {
			batch[i] = chunks[start+i].Content
			tracer.WithTrace(ctx).Infof("execute to %s embedding %d",chunks[start+i].FilePath, len(batch[i]))
		}

		

		// 执行嵌入
		embeddings, err := e.doEmbeddings(ctx, batch)
		if err != nil {
			tracer.WithTrace(ctx).Errorf("e.doEmbeddings(ctx, batch) filed: %v ",  err)
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
		for _,text := range texts {
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

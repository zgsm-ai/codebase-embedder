package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zgsm-ai/codebase-indexer/test/mocks"
	"github.com/zgsm-ai/codebase-indexer/test/types"
)

// EmbeddingTaskHandler 处理嵌入任务
type EmbeddingTaskHandler struct {
	vectorStore *mocks.MockVectorStore
	db          *mocks.MockDB
	redis       *mocks.MockRedis
}

// NewEmbeddingTaskHandler 创建新的嵌入任务处理器
func NewEmbeddingTaskHandler(vectorStore *mocks.MockVectorStore, db *mocks.MockDB, redis *mocks.MockRedis) *EmbeddingTaskHandler {
	return &EmbeddingTaskHandler{
		vectorStore: vectorStore,
		db:          db,
		redis:       redis,
	}
}

// CreateTask 创建嵌入任务
func (h *EmbeddingTaskHandler) CreateTask(ctx context.Context, req types.CreateEmbeddingRequest) (*types.CreateEmbeddingResponse, error) {
	// 验证参数
	if req.CodebasePath == "" {
		return nil, fmt.Errorf("代码库路径不能为空")
	}

	// 验证路径合法性
	if !isValidPath(req.CodebasePath) {
		return nil, fmt.Errorf("非法路径")
	}

	// 检查任务是否已存在
	lockKey := fmt.Sprintf("embedding_task_%s_%s", req.ClientID, req.CodebasePath)
	acquired, err := h.redis.AcquireLock(ctx, lockKey, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("获取锁失败: %v", err)
	}
	if !acquired {
		return nil, fmt.Errorf("任务已存在")
	}
	defer h.redis.ReleaseLock(ctx, lockKey)

	// 如果强制重建，删除现有索引
	if req.ForceRebuild {
		if err := h.db.DeleteExistingIndex(req.ClientID, req.CodebasePath); err != nil {
			return nil, fmt.Errorf("删除现有索引失败: %v", err)
		}
	}

	// 创建任务
	taskID, err := h.db.CreateIndexTask(ctx, &types.IndexTask{
		ClientId:     req.ClientID,
		CodebasePath: req.CodebasePath,
		Status:       "pending",
		Progress:     0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %v", err)
	}

	// 设置任务状态
	if err := h.redis.SetTaskStatus(ctx, taskID, "pending", 24*time.Hour); err != nil {
		return nil, fmt.Errorf("设置任务状态失败: %v", err)
	}

	return &types.CreateEmbeddingResponse{
		TaskID: taskID,
		Status: "pending",
	}, nil
}

// isValidPath 检查路径是否合法
func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	
	// 检查路径遍历
	if strings.Contains(path, "..") {
		return false
	}
	
	// 检查绝对路径
	if strings.HasPrefix(path, "/") || strings.Contains(path, ":\\") {
		return false
	}
	
	return true
}

// HTTPHandler HTTP处理器
func (h *EmbeddingTaskHandler) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	// 这里可以添加HTTP处理逻辑
}
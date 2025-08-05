package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

const (
	// Redis键前缀
	fileStatusPrefix = "file:status:"
	// 请求ID键前缀
	requestIdPrefix = "request:id:"
	// 默认过期时间 - 24小时
	defaultExpiration = 24 * time.Hour
)

// StatusManager 文件状态管理器
type StatusManager struct {
	client *redis.Client
}

// NewStatusManager 创建新的状态管理器
func NewStatusManager(client *redis.Client) *StatusManager {
	return &StatusManager{
		client: client,
	}
}

// SetFileStatusByRequestId 通过RequestId设置文件处理状态到Redis
func (sm *StatusManager) SetFileStatusByRequestId(ctx context.Context, requestId string, status *types.FileStatusResponseData) error {
	key := sm.generateRequestKey(requestId)
	logx.Infof("SetFileStatusByRequestId Key: %s", key)
	// 打印status日志
	logx.Infof("SetFileStatusByRequestId Status: %+v", status)
	// 序列化状态数据
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status data: %w", err)
	}

	// 设置到Redis，带过期时间
	err = sm.client.Set(ctx, key, data, defaultExpiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set status in redis: %w", err)
	}

	return nil
}

// GetFileStatus 从Redis获取文件处理状态
func (sm *StatusManager) GetFileStatus(ctx context.Context, requestId string) (*types.FileStatusResponseData, error) {
	key := sm.generateRequestKey(requestId)
	logx.Infof("GetFileStatus Key: %s", key)
	// 从Redis获取数据
	data, err := sm.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			// 键不存在，返回nil
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get status from redis: %w", err)
	}

	// 反序列化状态数据
	var status types.FileStatusResponseData
	err = json.Unmarshal([]byte(data), &status)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal status data: %w", err)
	}

	logx.Infof("3333333333333333333333333333333333")
	// 打印status日志
	logx.Infof("GetFileStatus Status: %+v", status)

	return &status, nil
}

func (sm *StatusManager) UpdateFileStatus(ctx context.Context, requestId string, updateFn func(*types.FileStatusResponseData)) error {
	key := sm.generateRequestKey(requestId)
	logx.Infof("UpdateFileStatus Key: %s", key)

	// 从Redis获取数据
	data, err := sm.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			// 键不存在，创建新的状态
			currentStatus := &types.FileStatusResponseData{
				Process:       "pending",
				TotalProgress: 0,
			}
			updateFn(currentStatus)
			return sm.SetFileStatusByRequestId(ctx, requestId, currentStatus)
		}
		return fmt.Errorf("failed to get status from redis: %w", err)
	}

	// 反序列化状态数据
	var currentStatus types.FileStatusResponseData
	err = json.Unmarshal([]byte(data), &currentStatus)
	if err != nil {
		return fmt.Errorf("failed to unmarshal status data: %w", err)
	}

	logx.Infof("UpdateFileStatus Status: %+v", currentStatus)

	// 应用更新函数
	updateFn(&currentStatus)

	// 保存更新后的状态
	return sm.SetFileStatusByRequestId(ctx, requestId, &currentStatus)
}

// DeleteFileStatus 删除文件处理状态
func (sm *StatusManager) DeleteFileStatus(ctx context.Context, clientID, codebasePath, codebaseName string) error {
	key := sm.generateKey(clientID, codebasePath, codebaseName)
	return sm.client.Del(ctx, key).Err()
}

// generateKey 生成Redis键
func (sm *StatusManager) generateKey(clientID, codebasePath, codebaseName string) string {
	// 使用clientID、codebasePath和codebaseName组合生成唯一键
	return fmt.Sprintf("%s%s:%s:%s", fileStatusPrefix, clientID, codebasePath, codebaseName)
}

// generateRequestKey 生成基于RequestId的Redis键
func (sm *StatusManager) generateRequestKey(requestId string) string {
	return fmt.Sprintf("%s%s", requestIdPrefix, requestId)
}

// generateSyncKey 生成基于syncId的Redis键
func (sm *StatusManager) generateSyncKey(syncId string) string {
	return fmt.Sprintf("%s%s", fileStatusPrefix, syncId)
}

// SetExpiration 设置自定义过期时间
func (sm *StatusManager) SetExpiration(ctx context.Context, clientID, codebasePath, codebaseName string, expiration time.Duration) error {
	key := sm.generateKey(clientID, codebasePath, codebaseName)
	return sm.client.Expire(ctx, key, expiration).Err()
}

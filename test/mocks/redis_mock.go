package mocks

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockRedis 是Redis接口的mock实现
type MockRedis struct {
	mock.Mock
}

// Set 设置键值
func (m *MockRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	args := m.Called(ctx, key, value, expiration)
	return args.Error(0)
}

// Get 获取键值
func (m *MockRedis) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

// Del 删除键
func (m *MockRedis) Del(ctx context.Context, keys ...string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

// Exists 检查键是否存在
func (m *MockRedis) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

// SetTaskStatus 设置任务状态
func (m *MockRedis) SetTaskStatus(ctx context.Context, taskID string, status string, expiration time.Duration) error {
	args := m.Called(ctx, taskID, status, expiration)
	return args.Error(0)
}

// GetTaskStatus 获取任务状态
func (m *MockRedis) GetTaskStatus(ctx context.Context, taskID string) (string, error) {
	args := m.Called(ctx, taskID)
	return args.String(0), args.Error(1)
}

// AcquireLock 获取分布式锁
func (m *MockRedis) AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	args := m.Called(ctx, key, expiration)
	return args.Bool(0), args.Error(1)
}

// ReleaseLock 释放分布式锁
func (m *MockRedis) ReleaseLock(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

// DeleteKeys 删除匹配的键
func (m *MockRedis) DeleteKeys(ctx context.Context, pattern string) (int64, error) {
	args := m.Called(ctx, pattern)
	return args.Get(0).(int64), args.Error(1)
}

// SetProgress 设置任务进度
func (m *MockRedis) SetProgress(ctx context.Context, taskID string, progress float64) error {
	args := m.Called(ctx, taskID, progress)
	return args.Error(0)
}

// GetProgress 获取任务进度
func (m *MockRedis) GetProgress(ctx context.Context, taskID string) (float64, error) {
	args := m.Called(ctx, taskID)
	return args.Get(0).(float64), args.Error(1)
}

// SetWithExpiration 设置带过期时间的键
func (m *MockRedis) SetWithExpiration(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	args := m.Called(ctx, key, value, expiration)
	return args.Error(0)
}

// GetWithExpiration 获取键并检查过期
func (m *MockRedis) GetWithExpiration(ctx context.Context, key string) (string, time.Duration, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Get(1).(time.Duration), args.Error(2)
}

// Increment 递增计数器
func (m *MockRedis) Increment(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

// Decrement 递减计数器
func (m *MockRedis) Decrement(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

// SetNX 设置键（如果不存在）
func (m *MockRedis) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	args := m.Called(ctx, key, value, expiration)
	return args.Bool(0), args.Error(1)
}

// MGet 批量获取键值
func (m *MockRedis) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	args := m.Called(ctx, keys)
	return args.Get(0).([]interface{}), args.Error(1)
}

// MSet 批量设置键值
func (m *MockRedis) MSet(ctx context.Context, values map[string]interface{}, expiration time.Duration) error {
	args := m.Called(ctx, values, expiration)
	return args.Error(0)
}

// Expire 设置键过期时间
func (m *MockRedis) Expire(ctx context.Context, key string, expiration time.Duration) error {
	args := m.Called(ctx, key, expiration)
	return args.Error(0)
}

// TTL 获取键剩余过期时间
func (m *MockRedis) TTL(ctx context.Context, key string) (time.Duration, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(time.Duration), args.Error(1)
}

// Keys 获取匹配的键
func (m *MockRedis) Keys(ctx context.Context, pattern string) ([]string, error) {
	args := m.Called(ctx, pattern)
	return args.Get(0).([]string), args.Error(1)
}

// FlushDB 清空当前数据库
func (m *MockRedis) FlushDB(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Ping 检查连接
func (m *MockRedis) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Close 关闭连接
func (m *MockRedis) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Subscribe 订阅频道
func (m *MockRedis) Subscribe(ctx context.Context, channel string) (interface{}, error) {
	args := m.Called(ctx, channel)
	return args.Get(0), args.Error(1)
}

// Publish 发布消息
func (m *MockRedis) Publish(ctx context.Context, channel string, message interface{}) error {
	args := m.Called(ctx, channel, message)
	return args.Error(0)
}

// SetHash 设置哈希表
func (m *MockRedis) SetHash(ctx context.Context, key string, values map[string]interface{}) error {
	args := m.Called(ctx, key, values)
	return args.Error(0)
}

// GetHash 获取哈希表
func (m *MockRedis) GetHash(ctx context.Context, key string) (map[string]string, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(map[string]string), args.Error(1)
}

// GetHashField 获取哈希表字段
func (m *MockRedis) GetHashField(ctx context.Context, key, field string) (string, error) {
	args := m.Called(ctx, key, field)
	return args.String(0), args.Error(1)
}

// DeleteHashField 删除哈希表字段
func (m *MockRedis) DeleteHashField(ctx context.Context, key string, fields ...string) error {
	args := m.Called(ctx, key, fields)
	return args.Error(0)
}

// AddToSet 添加到集合
func (m *MockRedis) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	args := m.Called(ctx, key, members)
	return args.Error(0)
}

// RemoveFromSet 从集合移除
func (m *MockRedis) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	args := m.Called(ctx, key, members)
	return args.Error(0)
}

// IsMember 检查成员是否在集合中
func (m *MockRedis) IsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	args := m.Called(ctx, key, member)
	return args.Bool(0), args.Error(1)
}

// GetSetMembers 获取集合所有成员
func (m *MockRedis) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]string), args.Error(1)
}

// GetSetSize 获取集合大小
func (m *MockRedis) GetSetSize(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

// AddToList 添加到列表
func (m *MockRedis) AddToList(ctx context.Context, key string, values ...interface{}) error {
	args := m.Called(ctx, key, values)
	return args.Error(0)
}

// GetListRange 获取列表范围
func (m *MockRedis) GetListRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	args := m.Called(ctx, key, start, stop)
	return args.Get(0).([]string), args.Error(1)
}

// GetListSize 获取列表长度
func (m *MockRedis) GetListSize(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

// RemoveFromList 从列表移除
func (m *MockRedis) RemoveFromList(ctx context.Context, key string, count int64, value interface{}) error {
	args := m.Called(ctx, key, count, value)
	return args.Error(0)
}

// SetBit 设置位图
func (m *MockRedis) SetBit(ctx context.Context, key string, offset int64, value int) error {
	args := m.Called(ctx, key, offset, value)
	return args.Error(0)
}

// GetBit 获取位图
func (m *MockRedis) GetBit(ctx context.Context, key string, offset int64) (int, error) {
	args := m.Called(ctx, key, offset)
	return args.Int(0), args.Error(1)
}

// BitCount 统计位图
func (m *MockRedis) BitCount(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

// SetHyperLogLog 设置HyperLogLog
func (m *MockRedis) SetHyperLogLog(ctx context.Context, key string, elements ...interface{}) error {
	args := m.Called(ctx, key, elements)
	return args.Error(0)
}

// CountHyperLogLog 统计HyperLogLog
func (m *MockRedis) CountHyperLogLog(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

// MergeHyperLogLog 合并HyperLogLog
func (m *MockRedis) MergeHyperLogLog(ctx context.Context, destKey string, sourceKeys ...string) error {
	args := m.Called(ctx, destKey, sourceKeys)
	return args.Error(0)
}

// SetGeoLocation 设置地理位置
func (m *MockRedis) SetGeoLocation(ctx context.Context, key string, longitude, latitude float64, member string) error {
	args := m.Called(ctx, key, longitude, latitude, member)
	return args.Error(0)
}

// GetGeoDistance 获取地理距离
func (m *MockRedis) GetGeoDistance(ctx context.Context, key, member1, member2 string, unit string) (float64, error) {
	args := m.Called(ctx, key, member1, member2, unit)
	return args.Get(0).(float64), args.Error(1)
}

// GetGeoRadius 获取地理半径内的成员
func (m *MockRedis) GetGeoRadius(ctx context.Context, key string, longitude, latitude, radius float64, unit string) ([]string, error) {
	args := m.Called(ctx, key, longitude, latitude, radius, unit)
	return args.Get(0).([]string), args.Error(1)
}

// Pipeline 创建管道
func (m *MockRedis) Pipeline() interface{} {
	args := m.Called()
	return args.Get(0)
}

// TxPipeline 创建事务管道
func (m *MockRedis) TxPipeline() interface{} {
	args := m.Called()
	return args.Get(0)
}

// Scan 扫描键
func (m *MockRedis) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	args := m.Called(ctx, cursor, match, count)
	return args.Get(0).([]string), args.Get(1).(uint64), args.Error(2)
}

// GetClientInfo 获取客户端信息
func (m *MockRedis) GetClientInfo() map[string]interface{} {
	args := m.Called()
	return args.Get(0).(map[string]interface{})
}

// GetServerInfo 获取服务器信息
func (m *MockRedis) GetServerInfo() map[string]string {
	args := m.Called()
	return args.Get(0).(map[string]string)
}
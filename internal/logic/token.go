package logic

import (
	"context"
	"errors"
	"fmt"
	"time"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// TokenLogic token生成逻辑
type TokenLogic struct {
	ctx    context.Context
}

// NewTokenLogic 创建TokenLogic实例
func NewTokenLogic(ctx context.Context) *TokenLogic {
	return &TokenLogic{
		ctx:    ctx,
	}
}

// GenerateToken 生成JWT令牌
func (l *TokenLogic) GenerateToken(req *types.TokenRequest) (*types.TokenResponseData, error) {
	if err := l.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// 使用clientId和codebasePath生成token
	// 这里使用简单的哈希组合，实际生产环境应使用更安全的JWT实现
	token := fmt.Sprintf("%s_%s_%s", req.ClientId, req.CodebasePath, l.generateRandomString(16))

	return &types.TokenResponseData{
		Token:     token,
		ExpiresIn: 3600, // 1小时 = 3600秒
		TokenType: "Bearer",
	}, nil
}

// validateRequest 验证请求参数
func (l *TokenLogic) validateRequest(req *types.TokenRequest) error {
	if req.ClientId == "" {
		return errors.New("clientId is required")
	}
	if req.CodebasePath == "" {
		return errors.New("codebasePath is required")
	}
	if req.CodebaseName == "" {
		return errors.New("codebaseName is required")
	}
	return nil
}

// generateJTI 生成唯一的JWT ID
func (l *TokenLogic) generateJTI() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), l.generateRandomString(8))
}

// generateRandomString 生成随机字符串
func (l *TokenLogic) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	
	// 使用更稳定的随机源
	seed := time.Now().UnixNano()
	for i := range result {
		seed = (seed * 1103515245 + 12345) & 0x7fffffff
		result[i] = charset[seed%int64(len(charset))]
	}
	return string(result)
}

// getSecretKey 获取JWT签名密钥
func (l *TokenLogic) getSecretKey() string {
	// 默认密钥（仅用于开发环境）
	return "default-secret-key-change-in-production"
}

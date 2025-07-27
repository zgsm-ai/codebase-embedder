package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zgsm-ai/codebase-indexer/internal/logic"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// TokenAuthMiddleware token认证中间件
type TokenAuthMiddleware struct {
	svcCtx *svc.ServiceContext
}

// NewTokenAuthMiddleware 创建token认证中间件
func NewTokenAuthMiddleware(svcCtx *svc.ServiceContext) *TokenAuthMiddleware {
	return &TokenAuthMiddleware{
		svcCtx: svcCtx,
	}
}

// Handle 处理token认证
func (m *TokenAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 从Authorization头获取token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			httpx.Error(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		// 解析Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			httpx.Error(w, http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		tokenString := parts[1]
		if tokenString == "" {
			httpx.Error(w, http.StatusUnauthorized, "Token is required")
			return
		}

		// 验证token
		tokenLogic := logic.NewTokenLogic(r.Context(), m.svcCtx.Config)
		token, err := tokenLogic.ValidateToken(tokenString)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, err.Error())
			return
		}

		// 提取claims并设置到context
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "Invalid token claims")
			return
		}

		// 将客户端信息设置到context中
		ctx := context.WithValue(r.Context(), "tokenClaims", claims)
		next(w, r.WithContext(ctx))
	}
}

// GetTokenClaims 从context中获取token claims
func GetTokenClaims(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value("tokenClaims").(jwt.MapClaims)
	return claims, ok
}
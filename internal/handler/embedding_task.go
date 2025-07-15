package handler

import (
	"github.com/zgsm-ai/codebase-indexer/internal/response"
	"net/http"
	"errors"
	// "github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zgsm-ai/codebase-indexer/internal/logic"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

// SubmitTask 处理任务提交请求
// @Summary 提交任务
// func taskHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		var req types.IndexTaskRequest
// 		if err := httpx.Parse(r, &req); err != nil {
// 			response.Error(w, err)
// 			return
// 		}

// 		l := logic.NewTaskLogic(r.Context(), svcCtx)
// 		resp, err := l.SubmitTask(&req, r)
// 		if err != nil {
// 			response.Error(w, err)
// 		} else {
// 			response.Json(w, resp)
// 		}
// 	}
// }


func taskHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.IndexTaskRequest
		// 修改解析逻辑，从form-data解析参数
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 最大32MB
			response.Error(w, err)
			return
		}

		// 手动映射form参数到请求结构体
		req.ClientId = r.FormValue("clientId")
		req.CodebasePath = r.FormValue("codebasePath")
		req.CodebaseName = r.FormValue("codebaseName")

		// 验证必填字段
		if req.ClientId == "" {
			response.Error(w, errors.New("missing required parameter: clientId"))
			return
		}
		if req.CodebasePath == "" {
			response.Error(w, errors.New("missing required parameter: codebasePath"))
			return
		}
		if req.CodebaseName == "" {
			response.Error(w, errors.New("missing required parameter: codebaseName"))
			return
		}


		l := logic.NewTaskLogic(r.Context(), svcCtx)
		resp, err := l.SubmitTask(&req, r)
		if err != nil {
			response.Error(w, err)
		} else {
			response.Json(w, resp)
		}
	}
}
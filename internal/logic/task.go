package logic

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"github.com/zgsm-ai/codebase-indexer/internal/dao/model"
	"github.com/zgsm-ai/codebase-indexer/internal/job"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"github.com/zgsm-ai/codebase-indexer/pkg/utils"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/zgsm-ai/codebase-indexer/internal/errs"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
	"github.com/zgsm-ai/codebase-indexer/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type TaskLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TaskLogic {
	return &TaskLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *TaskLogic) SubmitTask(req *types.IndexTaskRequest, r *http.Request) (resp *types.IndexTaskResponseData, err error) {
	clientId := req.ClientId
	clientPath := req.CodebasePath
	codebaseName := req.CodebaseName
	l.Logger.Debugf("SubmitTask request: %s, %s, %s", clientId, clientPath, codebaseName)

	userUid := utils.ParseJWTUserInfo(r, l.svcCtx.Config.Auth.UserInfoHeader)

	// 查找代码库记录，不存在则初始化
	codebase, err := l.initCodebaseIfNotExists(clientId, clientPath, userUid, codebaseName)
	if err != nil {
		return nil, err
	}

	// 创建索引任务
	// 查询最新的同步
	latestSync, err := l.svcCtx.Querier.SyncHistory.FindLatest(l.ctx, codebase.ID)
	if err != nil {
		return nil, errs.NewRecordNotFoundErr(types.NameSyncHistory, fmt.Sprintf("codebase_id: %d", codebase.ID))
	}
	ctx := context.WithValue(l.ctx, tracer.Key, tracer.RequestTraceId(int(codebase.ID)))

	// 获取同步锁，避免重复处理
	// 获取分布式锁， n分钟超时
	lockKey := fmt.Sprintf("codebase_embedder:task:%d", codebase.ID)

	mux, locked, err := l.svcCtx.DistLock.TryLock(ctx, lockKey, l.svcCtx.Config.IndexTask.LockTimeout)
	if err != nil || !locked {
		return nil, fmt.Errorf("failed to acquire lock %s to sumit index task, err:%w", lockKey, err)
	}
	defer l.svcCtx.DistLock.Unlock(ctx, mux)

	tracer.WithTrace(ctx).Infof("acquire lock %s successfully, start to submit index task.", lockKey)

	// TODO 从body 中读取文件
	// Parse multipart form
	err = r.ParseMultipartForm(32 << 20) // 32MB max memory
	if err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	defer r.MultipartForm.RemoveAll()

	// Get the ZIP file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("failed to get file from form: %w", err)
	}
	defer file.Close()

	// Verify file is a ZIP
	if !strings.HasSuffix(header.Filename, ".zip") {
		return nil, fmt.Errorf("uploaded file must be a ZIP file, got: %s", header.Filename)
	}

	// 补全代码：从ZIP文件读取所有文件内容
	files := make(map[string][]byte)

	// 创建临时文件存储上传的ZIP
	tempFile, err := os.CreateTemp("", "upload-*.zip")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath) // 清理临时文件

	// 将上传的ZIP内容复制到临时文件
	_, err = io.Copy(tempFile, file)
	tempFile.Close() // 关闭文件以便后续读取
	if err != nil {
		return nil, fmt.Errorf("failed to copy file to temp location: %w", err)
	}

	// 打开ZIP文件进行读取
	zipReader, err := zip.OpenReader(tempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP file: %w", err)
	}
	defer zipReader.Close()

	// 遍历ZIP中的所有文件
	for _, zipFile := range zipReader.File {
		// 跳过目录
		if zipFile.FileInfo().IsDir() {
			continue
		}

		// 打开ZIP中的文件
		fileReader, err := zipFile.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s in zip: %w", zipFile.Name, err)
		}

		// 读取文件内容
		content, err := io.ReadAll(fileReader)
		fileReader.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s in zip: %w", zipFile.Name, err)
		}

		// 存储文件内容到映射
		files[zipFile.Name] = content
	}

	task := &job.IndexTask{
		SvcCtx:  l.svcCtx,
		LockMux: mux,
		Params: &job.IndexTaskParams{
			SyncID:       latestSync.ID,
			CodebaseID:   codebase.ID,
			CodebasePath: codebase.Path,
			CodebaseName: codebase.Name,
			Files:        files,
		},
	}

	err = l.svcCtx.TaskPool.Submit(func() {
		taskTimeout, cancelFunc := context.WithTimeout(context.Background(), l.svcCtx.Config.IndexTask.GraphTask.Timeout)
		traceCtx := context.WithValue(taskTimeout, tracer.Key, tracer.TaskTraceId(int(codebase.ID)))
		defer cancelFunc()
		task.Run(traceCtx)
	})

	if err != nil {
		return nil, fmt.Errorf("index task submit failed, err:%w", err)
	}
	tracer.WithTrace(ctx).Infof("index task submit successfully.")

	return &types.IndexTaskResponseData{}, nil
}

func (l *TaskLogic) initCodebaseIfNotExists(clientId, clientPath, userUid, codebaseName string) (*model.Codebase, error) {
	var codebase *model.Codebase
	var err error
	// 判断数据库记录是否存在 ，状态为 active
	codebase, err = l.svcCtx.Querier.Codebase.FindByClientIdAndPath(l.ctx, clientId, clientPath)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if codebase == nil {
		codebase, err = l.saveCodebase(clientId, clientPath, userUid, codebaseName)
	}

	return codebase, nil
}

/**
 * @Description: 初始化 codebase
 * @receiver l
 * @param clientId
 * @param clientPath
 * @param r
 * @param codebaseName
 * @param metadata
 * @return error
 * @return bool
 */
func (l *TaskLogic) saveCodebase(clientId, clientPath, userUId, codebaseName string) (*model.Codebase, error) {
	// 不存在则插入
	// clientId + codebasepath 为联合唯一索引
	// 保存到数据库
	codebaseModel := &model.Codebase{
		ClientID:   clientId,
		UserID:     userUId,
		Name:       codebaseName,
		ClientPath: clientPath,
		Status:     string(model.CodebaseStatusActive),
		Path:       clientPath,
	}
	err := l.svcCtx.Querier.Codebase.WithContext(l.ctx).Save(codebaseModel)
	if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		// 不是 唯一索引冲突
		return nil, err
	}
	return codebaseModel, nil
}

package logic

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-redsync/redsync/v4"
	"github.com/zgsm-ai/codebase-indexer/internal/dao/model"
	"github.com/zgsm-ai/codebase-indexer/internal/job"
	"github.com/zgsm-ai/codebase-indexer/internal/tracer"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
	"github.com/zgsm-ai/codebase-indexer/pkg/utils"
	"gorm.io/gorm"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zgsm-ai/codebase-indexer/internal/svc"
)

type TaskLogic struct {
	logx.Logger
	ctx          context.Context
	svcCtx       *svc.ServiceContext
	syncMetadata *types.SyncMetadata
}

func NewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TaskLogic {
	return &TaskLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
		syncMetadata: &types.SyncMetadata{
			ClientId:      "",
			CodebasePath:  "",
			CodebaseName:  "",
			ExtraMetadata: make(map[string]interface{}),
			FileList:      make(map[string]string),
			Timestamp:     0,
		},
	}
}

func (l *TaskLogic) SubmitTask(req *types.IndexTaskRequest, r *http.Request) (resp *types.IndexTaskResponseData, err error) {
	clientId := req.ClientId
	clientPath := req.CodebasePath
	codebaseName := req.CodebaseName
	uploadToken := req.UploadToken

	l.Logger.Debugf("SubmitTask request: %s, %s, %s, uploadToken: %s", clientId, clientPath, codebaseName, uploadToken)

	// 验证uploadToken的有效性
	if err := l.validateUploadToken(uploadToken); err != nil {
		return nil, err
	}

	userUid := utils.ParseJWTUserInfo(r, l.svcCtx.Config.Auth.UserInfoHeader)

	// 查找代码库记录，不存在则初始化
	codebase, err := l.initCodebaseIfNotExists(clientId, clientPath, userUid, codebaseName)
	if err != nil {
		return nil, err
	}

	ctx := context.WithValue(l.ctx, tracer.Key, tracer.RequestTraceId(int(codebase.ID)))

	// 获取分布式锁
	mux, err := l.acquireTaskLock(ctx, codebase.ID)
	if err != nil {
		return nil, err
	}
	defer l.svcCtx.DistLock.Unlock(ctx, mux)

	// 处理上传的ZIP文件
	files, fileCount, metadata, err := l.processUploadedZipFile(r)
	if err != nil {
		return nil, err
	}

	// 更新代码库信息
	if err := l.updateCodebaseInfo(codebase, fileCount, int64(req.FileTotals)); err != nil {
		return nil, err
	}

	// 生成请求ID
	requestId := l.generateRequestId(req.RequestId, clientId, codebase.Name)

	// 提交索引任务
	if err := l.submitIndexTask(ctx, codebase, clientId, requestId, mux, files, metadata); err != nil {
		return nil, err
	}

	// 初始化文件处理状态
	if err := l.initializeFileStatus(ctx, req.RequestId); err != nil {
		l.Logger.Errorf("failed to set initial file status in redis with requestId %s: %v", req.RequestId, err)
		// 不返回错误，继续处理
	}

	return &types.IndexTaskResponseData{TaskId: int(codebase.ID)}, nil
}

// validateUploadToken 验证上传令牌的有效性
func (l *TaskLogic) validateUploadToken(uploadToken string) error {
	// TODO: 验证uploadToken的有效性
	// 当前调试阶段，万能令牌"xxxx"直接通过
	if uploadToken != "xxxx" {
		// 这里可以添加真实的token验证逻辑
		// l.Logger.Warnf("Invalid upload token: %s", uploadToken)
	}
	return nil
}

// acquireTaskLock 获取任务锁
func (l *TaskLogic) acquireTaskLock(ctx context.Context, codebaseID int32) (*redsync.Mutex, error) {
	lockKey := fmt.Sprintf("codebase_embedder:task:%d", codebaseID)

	mux, locked, err := l.svcCtx.DistLock.TryLock(ctx, lockKey, l.svcCtx.Config.IndexTask.LockTimeout)
	if err != nil || !locked {
		return nil, fmt.Errorf("failed to acquire lock %s to sumit index task, err:%w", lockKey, err)
	}

	tracer.WithTrace(ctx).Infof("acquire lock %s successfully, start to submit index task.", lockKey)
	return mux, nil
}

// processUploadedZipFile 处理上传的ZIP文件
func (l *TaskLogic) processUploadedZipFile(r *http.Request) (map[string][]byte, int, *types.SyncMetadata, error) {
	// 解析multipart表单
	err := r.ParseMultipartForm(32 << 20) // 32MB max memory
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	defer r.MultipartForm.RemoveAll()

	// 从表单中获取ZIP文件
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to get file from form: %w", err)
	}
	defer file.Close()

	// 验证文件是否为ZIP格式
	if !strings.HasSuffix(header.Filename, ".zip") {
		return nil, 0, nil, fmt.Errorf("uploaded file must be a ZIP file, got: %s", header.Filename)
	}

	// 处理ZIP文件内容
	return l.extractZipFiles(file)
}

// extractZipFiles 从ZIP文件中提取文件内容
func (l *TaskLogic) extractZipFiles(file io.Reader) (map[string][]byte, int, *types.SyncMetadata, error) {
	files := make(map[string][]byte)

	// 创建临时文件存储上传的ZIP
	tempFile, err := os.CreateTemp("", "upload-*.zip")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	// defer os.Remove(tempPath) // 清理临时文件

	tracer.WithTrace(l.ctx).Infof("extractZipFiles tempPath %s", tempPath)

	// 将上传的ZIP内容复制到临时文件
	_, err = io.Copy(tempFile, file)
	tempFile.Close() // 关闭文件以便后续读取
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to copy file to temp location: %w", err)
	}

	// 打开ZIP文件进行读取
	zipReader, err := zip.OpenReader(tempPath)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to open ZIP file: %w", err)
	}
	defer zipReader.Close()

	// 检查是否存在.shenma_sync文件夹
	if !l.hasShenmaSyncFolder(zipReader) {
		return nil, 0, nil, fmt.Errorf("ZIP文件中必须包含.shenma_sync文件夹")
	}

	// 提取文件内容
	fileCount, err := l.extractFilesFromZip(zipReader, files)
	if err != nil {
		return nil, 0, nil, err
	}

	// 获取元数据
	metadata := l.getSyncMetadata()

	return files, fileCount, metadata, nil
}

// hasShenmaSyncFolder 检查ZIP中是否存在.shenma_sync文件夹
func (l *TaskLogic) hasShenmaSyncFolder(zipReader *zip.ReadCloser) bool {
	for _, zipFile := range zipReader.File {
		if strings.HasPrefix(zipFile.Name, ".shenma_sync/") {
			return true
		}
	}
	return false
}

// extractFilesFromZip 从ZIP中提取文件内容
func (l *TaskLogic) extractFilesFromZip(zipReader *zip.ReadCloser, files map[string][]byte) (int, error) {
	fileCount := 0
	shenmaSyncFiles := make(map[string][]byte)

	// 遍历ZIP中的所有文件
	for _, zipFile := range zipReader.File {
		// 跳过目录
		if zipFile.FileInfo().IsDir() {
			continue
		}

		// 处理.shenma_sync文件夹中的文件
		if strings.HasPrefix(zipFile.Name, ".shenma_sync/") {
			if err := l.processShenmaSyncFile(zipFile, shenmaSyncFiles); err != nil {
				return 0, err
			}
			continue
		}

		// 处理普通文件
		fileCount++
		if err := l.processRegularFile(zipFile, files); err != nil {
			return 0, err
		}
	}

	// 打印.shenma_sync文件夹中的文件摘要
	l.Logger.Infof("共找到 %d 个.shenma_sync文件夹中的文件", len(shenmaSyncFiles))
	for fileName := range shenmaSyncFiles {
		l.Logger.Infof(" - %s", fileName)
	}

	return fileCount, nil
}

// processShenmaSyncFile 处理.shenma_sync文件夹中的文件
func (l *TaskLogic) processShenmaSyncFile(zipFile *zip.File, shenmaSyncFiles map[string][]byte) error {
	fileReader, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %s in zip: %w", zipFile.Name, err)
	}

	content, err := io.ReadAll(fileReader)
	fileReader.Close()
	if err != nil {
		return fmt.Errorf("failed to read file %s in zip: %w", zipFile.Name, err)
	}

	shenmaSyncFiles[zipFile.Name] = content
	l.Logger.Infof("读取.shenma_sync文件夹中的文件: %s", zipFile.Name)
	l.Logger.Infof("文件内容:\n%s", string(content))

	// 解析JSON格式的.shenma_sync文件内容并提取fileList
	l.extractFileListFromShenmaSync(content, zipFile.Name)

	// 额外输出到控制台，确保用户能看到
	fmt.Printf("=== .shenma_sync文件内容 ===\n")
	fmt.Printf("文件名: %s\n", zipFile.Name)
	fmt.Printf("内容长度: %d 字节\n", len(content))
	fmt.Printf("内容:\n%s\n", string(content))
	fmt.Printf("========================\n\n")

	return nil
}

// extractFileListFromShenmaSync 从.shenma_sync文件内容中提取fileList
func (l *TaskLogic) extractFileListFromShenmaSync(content []byte, fileName string) {
	// 解析JSON内容
	var metadata types.SyncMetadata
	metadata.FileList = make(map[string]string)
	metadata.ExtraMetadata = make(map[string]interface{})

	if err := json.Unmarshal(content, &metadata); err != nil {
		l.Logger.Errorf("解析.shenma_sync文件失败 %s: %v", fileName, err)
		return
	}

	l.Logger.Infof("从 %s 中提取到 %d 个文件:", fileName, len(metadata.FileList))

	// 打印fileList中的文件
	for filePath, status := range metadata.FileList {
		l.Logger.Infof("  文件: %s, 状态: %s", filePath, status)
		fmt.Printf("  文件: %s, 状态: %s\n", filePath, status)
	}

	// 存储提取的元数据
	l.syncMetadata = &metadata
}

// processRegularFile 处理常规文件
func (l *TaskLogic) processRegularFile(zipFile *zip.File, files map[string][]byte) error {
	fileReader, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %s in zip: %w", zipFile.Name, err)
	}

	content, err := io.ReadAll(fileReader)
	fileReader.Close()
	if err != nil {
		return fmt.Errorf("failed to read file %s in zip: %w", zipFile.Name, err)
	}

	// 存储文件内容到映射
	files[zipFile.Name] = content
	return nil
}

// updateCodebaseInfo 更新代码库信息
func (l *TaskLogic) updateCodebaseInfo(codebase *model.Codebase, fileCount int, fileTotals int64) error {
	// 更新codebase的file_count和total_size字段
	codebase.FileCount = int32(fileCount)
	codebase.TotalSize = fileTotals
	err := l.svcCtx.Querier.Codebase.WithContext(l.ctx).Save(codebase)
	if err != nil {
		return fmt.Errorf("failed to update codebase file count: %w", err)
	}

	l.Logger.Infof("Updated codebase %d with file_count: %d, total_size: %d", codebase.ID, fileCount, fileTotals)
	return nil
}

// generateRequestId 生成请求ID
func (l *TaskLogic) generateRequestId(requestId, clientId, codebaseName string) string {
	if requestId == "" {
		return fmt.Sprintf("%s-%s-%d", clientId, codebaseName, time.Now().Unix())
	}
	return requestId
}

// submitIndexTask 提交索引任务
func (l *TaskLogic) submitIndexTask(ctx context.Context, codebase *model.Codebase, clientId, requestId string, mux *redsync.Mutex, files map[string][]byte, metadata *types.SyncMetadata) error {
	task := &job.IndexTask{
		SvcCtx:  l.svcCtx,
		LockMux: mux,
		Params: &job.IndexTaskParams{
			ClientId:     clientId,
			CodebaseID:   codebase.ID,
			CodebasePath: codebase.Path,
			CodebaseName: codebase.Name,
			RequestId:    requestId,
			Files:        files,
			Metadata:     metadata,
		},
	}

	err := l.svcCtx.TaskPool.Submit(func() {
		taskTimeout, cancelFunc := context.WithTimeout(context.Background(), l.svcCtx.Config.IndexTask.GraphTask.Timeout)
		traceCtx := context.WithValue(taskTimeout, tracer.Key, tracer.TaskTraceId(int(codebase.ID)))
		defer cancelFunc()
		task.Run(traceCtx)
	})

	if err != nil {
		return fmt.Errorf("index task submit failed, err:%w", err)
	}

	tracer.WithTrace(ctx).Infof("index task submit successfully.")
	return nil
}

// initializeFileStatus 初始化文件处理状态
func (l *TaskLogic) initializeFileStatus(ctx context.Context, requestId string) error {
	initialStatus := &types.FileStatusResponseData{
		Process:       "pending",
		TotalProgress: 0,
		FileList:      []types.FileStatusItem{},
	}

	// 使用RequestId作为键存储状态
	return l.svcCtx.StatusManager.SetFileStatusByRequestId(ctx, requestId, initialStatus)
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

// getSyncMetadata 获取同步元数据
func (l *TaskLogic) getSyncMetadata() *types.SyncMetadata {
	// 返回从ZIP文件中提取的元数据
	return l.syncMetadata
}

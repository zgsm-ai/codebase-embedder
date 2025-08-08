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
	"github.com/zgsm-ai/codebase-indexer/internal/store/vector"
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
			ExtraMetadata: make(map[string]types.MetadataValue),
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
	// mux, err := l.acquireTaskLock(ctx, codebase.ID)
	mux, err := l.acquireTaskLock(ctx, req.RequestId)
	if err != nil {
		return nil, err
	}
	defer l.svcCtx.DistLock.Unlock(ctx, mux)

	// 处理上传的ZIP文件
	files, fileCount, metadata, err := l.processUploadedZipFile(r)
	if err != nil {
		return nil, err
	}

	// 遍历任务并分类
	var addTasks, deleteTasks, modifyTasks []string
	if l.syncMetadata != nil {
		for key, value := range l.syncMetadata.FileList {

			switch strings.ToLower(value) {
			case "add":
				addTasks = append(addTasks, key)
			case "delete":
				deleteTasks = append(deleteTasks, key)
			case "modify":
				modifyTasks = append(modifyTasks, key)
			default:
				l.Logger.Errorf("未知的操作类型 %s 对于文件 %s", value, key)
			}
		}
	}

	// 记录任务分类统计
	l.Logger.Infof("任务分类统计 - 添加: %d, 删除: %d, 修改: %d",
		len(addTasks), len(deleteTasks), len(modifyTasks))

	// 如果有删除任务，从向量数据库中删除对应的文件
	if len(deleteTasks) > 0 {
		l.Logger.Infof("开始从向量数据库中删除 %d 个文件", len(deleteTasks))
		if err := l.deleteFilesFromVectorDB(ctx, codebase, deleteTasks); err != nil {
			l.Logger.Errorf("从向量数据库删除文件失败: %v", err)
			// 不返回错误，继续处理其他任务
		} else {
			l.Logger.Infof("成功从向量数据库中删除 %d 个文件", len(deleteTasks))
		}
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

	return &types.IndexTaskResponseData{TaskId: req.RequestId}, nil
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
func (l *TaskLogic) acquireTaskLock(ctx context.Context, codebaseID string) (*redsync.Mutex, error) {
	lockKey := fmt.Sprintf("codebase_embedder:task:%s", codebaseID)

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

	// 先找到控制源文件
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
			break
		}
	}

	// 遍历ZIP中的所有文件
	for _, zipFile := range zipReader.File {
		// 跳过目录
		if zipFile.FileInfo().IsDir() {
			continue
		}

		// 检查文件是否存在于ExtraMetadata中，如果不存在则忽略
		if l.syncMetadata != nil {
			// 将zipFile.Name中的Windows路径格式（反斜杠\）转换为Linux路径格式（正斜杠/）
			linuxPath := strings.ReplaceAll(zipFile.Name, "\\", "/")
			if _, exists := l.syncMetadata.FileList[linuxPath]; !exists {
				// l.Logger.Infof("文件 %s 不存在于syncMetadata.FileList中，跳过处理 %v", zipFile.Name, l.syncMetadata.FileList)
				continue
			} else {
				if l.syncMetadata.FileList[linuxPath] == "delete" {
					continue
				}
			}
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
	metadata.ExtraMetadata = make(map[string]types.MetadataValue)

	// 先解析为通用类型，然后转换为MetadataValue
	var tempMetadata struct {
		ClientId      string                 `json:"clientId"`
		CodebasePath  string                 `json:"codebasePath"`
		CodebaseName  string                 `json:"codebaseName"`
		ExtraMetadata map[string]interface{} `json:"extraMetadata"`
		FileList      map[string]string      `json:"fileList"`
		Timestamp     int64                  `json:"timestamp"`
	}

	if err := json.Unmarshal(content, &tempMetadata); err != nil {
		l.Logger.Errorf("解析.shenma_sync文件失败 %s: %v", fileName, err)
		return
	}

	// 转换ExtraMetadata的类型
	for key, value := range tempMetadata.ExtraMetadata {
		switch v := value.(type) {
		case string:
			metadata.ExtraMetadata[key] = types.NewStringMetadataValue(v)
		case float64:
			metadata.ExtraMetadata[key] = types.NewNumberMetadataValue(v)
		case bool:
			metadata.ExtraMetadata[key] = types.NewBoolMetadataValue(v)
		case []interface{}:
			// 处理数组类型
			if len(v) > 0 {
				switch v[0].(type) {
				case string:
					strSlice := make([]string, len(v))
					for i, elem := range v {
						strSlice[i] = elem.(string)
					}
					metadata.ExtraMetadata[key] = types.NewStringArrayMetadataValue(strSlice)
				case float64:
					numSlice := make([]float64, len(v))
					for i, elem := range v {
						numSlice[i] = elem.(float64)
					}
					metadata.ExtraMetadata[key] = types.NewNumberArrayMetadataValue(numSlice)
				}
			}
		}
	}

	metadata.ClientId = tempMetadata.ClientId
	metadata.CodebasePath = tempMetadata.CodebasePath
	metadata.CodebaseName = tempMetadata.CodebaseName
	metadata.FileList = tempMetadata.FileList
	metadata.Timestamp = tempMetadata.Timestamp

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
			TotalFiles:   len(files),
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

// deleteFilesFromVectorDB 从向量数据库中删除指定的文件
func (l *TaskLogic) deleteFilesFromVectorDB(ctx context.Context, codebase *model.Codebase, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil // 没有文件需要删除
	}

	l.Logger.Infof("准备从向量数据库中删除 %d 个文件，代码库ID: %d", len(filePaths), codebase.ID)

	// 构建需要删除的 CodeChunk 列表
	var chunks []*types.CodeChunk
	for _, filePath := range filePaths {
		// 将文件路径转换为Linux格式（正斜杠）
		linuxPath := strings.ReplaceAll(filePath, "\\", "/")
		chunk := &types.CodeChunk{
			CodebaseId:   codebase.ID,
			CodebasePath: codebase.Path,
			FilePath:     linuxPath,
		}
		chunks = append(chunks, chunk)
		l.Logger.Debugf("添加文件到删除列表: %s", linuxPath)
	}

	// 调用向量数据库的删除方法
	options := vector.Options{
		CodebasePath: codebase.Path,
	}
	err := l.svcCtx.VectorStore.DeleteCodeChunks(ctx, chunks, options)
	if err != nil {
		l.Logger.Errorf("删除向量数据库中的文件失败: %v", err)
		return fmt.Errorf("failed to delete files from vector database: %w", err)
	}

	l.Logger.Infof("成功从向量数据库中删除了 %d 个文件", len(filePaths))
	return nil
}

// getSyncMetadata 获取同步元数据
func (l *TaskLogic) getSyncMetadata() *types.SyncMetadata {
	// 返回从ZIP文件中提取的元数据
	return l.syncMetadata
}

package controller

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"webssh/core"

	"github.com/gin-gonic/gin"
)

type resumableUploadSession struct {
	ID          string
	ClientKey   string
	SSHInfo     string
	Path        string
	Dir         string
	FileName    string
	FileSize    int64
	ChunkSize   int64
	TotalChunks int
	Received    map[int]struct{}
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type resumableStore struct {
	mu           sync.RWMutex
	byID         map[string]*resumableUploadSession
	fileIDByKey  map[string]string
}

var uploadStore = &resumableStore{
	byID:        make(map[string]*resumableUploadSession),
	fileIDByKey: make(map[string]string),
}

var cleanupOnce sync.Once

const (
	resumableExpireAfter = 24 * time.Hour
	resumableCleanupTick = 30 * time.Minute
)

func resumableBaseDir() string {
	return filepath.Join(os.TempDir(), "webssh-upload-chunks")
}

func resumableSessionDir(fileID string) string {
	return filepath.Join(resumableBaseDir(), fileID)
}

func resumableChunkPath(fileID string, index int) string {
	return filepath.Join(resumableSessionDir(fileID), fmt.Sprintf("%06d.part", index))
}

func uploadSessionID(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func parseChunkList(m map[int]struct{}) []int {
	list := make([]int, 0, len(m))
	for idx := range m {
		list = append(list, idx)
	}
	sort.Ints(list)
	return list
}

func removeSession(fileID string) {
	uploadStore.mu.Lock()
	defer uploadStore.mu.Unlock()
	if s, ok := uploadStore.byID[fileID]; ok {
		delete(uploadStore.fileIDByKey, s.ClientKey)
	}
	delete(uploadStore.byID, fileID)
}

func startResumableCleaner() {
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(resumableCleanupTick)
			defer ticker.Stop()
			for range ticker.C {
				now := time.Now()
				var expired []string
				uploadStore.mu.RLock()
				for id, s := range uploadStore.byID {
					if now.Sub(s.UpdatedAt) >= resumableExpireAfter {
						expired = append(expired, id)
					}
				}
				uploadStore.mu.RUnlock()
				for _, id := range expired {
					removeSession(id)
					_ = os.RemoveAll(resumableSessionDir(id))
				}
			}
		}()
	})
}

type uploadInitReq struct {
	ClientKey   string `json:"clientKey" form:"clientKey"`
	SSHInfo     string `json:"sshInfo" form:"sshInfo"`
	Path        string `json:"path" form:"path"`
	Dir         string `json:"dir" form:"dir"`
	FileName    string `json:"fileName" form:"fileName"`
	FileSize    int64  `json:"fileSize" form:"fileSize"`
	ChunkSize   int64  `json:"chunkSize" form:"chunkSize"`
	TotalChunks int    `json:"totalChunks" form:"totalChunks"`
}

// UploadInit creates/resumes a chunk upload session.
func UploadInit(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	startResumableCleaner()

	var req uploadInitReq
	if err := c.ShouldBind(&req); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	req.Path = strings.TrimSpace(req.Path)
	req.Dir = strings.Trim(req.Dir, "/")
	req.FileName = strings.TrimSpace(req.FileName)
	req.ClientKey = strings.TrimSpace(req.ClientKey)

	if req.SSHInfo == "" || req.FileName == "" || req.FileSize < 0 || req.ChunkSize <= 0 || req.TotalChunks <= 0 {
		responseBody.Msg = "invalid upload init params"
		return &responseBody
	}
	if req.ClientKey == "" {
		req.ClientKey = fmt.Sprintf("%s:%d:%s:%s", req.FileName, req.FileSize, req.Path, req.Dir)
	}

	seed := strings.Join([]string{
		req.ClientKey,
		req.SSHInfo,
		req.Path,
		req.Dir,
		req.FileName,
		strconv.FormatInt(req.FileSize, 10),
		strconv.FormatInt(req.ChunkSize, 10),
		strconv.Itoa(req.TotalChunks),
	}, "|")
	fileID := uploadSessionID(seed)

	uploadStore.mu.Lock()
	defer uploadStore.mu.Unlock()

	if oldID, ok := uploadStore.fileIDByKey[req.ClientKey]; ok {
		if session, ok2 := uploadStore.byID[oldID]; ok2 {
			responseBody.Data = gin.H{
				"fileId":        session.ID,
				"uploadedChunks": parseChunkList(session.Received),
				"totalChunks":   session.TotalChunks,
			}
			return &responseBody
		}
	}

	session := &resumableUploadSession{
		ID:          fileID,
		ClientKey:   req.ClientKey,
		SSHInfo:     req.SSHInfo,
		Path:        req.Path,
		Dir:         req.Dir,
		FileName:    req.FileName,
		FileSize:    req.FileSize,
		ChunkSize:   req.ChunkSize,
		TotalChunks: req.TotalChunks,
		Received:    make(map[int]struct{}),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := os.MkdirAll(resumableSessionDir(fileID), 0o755); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}

	uploadStore.byID[fileID] = session
	uploadStore.fileIDByKey[req.ClientKey] = fileID
	responseBody.Data = gin.H{
		"fileId":        fileID,
		"uploadedChunks": []int{},
		"totalChunks":   req.TotalChunks,
	}
	return &responseBody
}

// UploadAbort cancels an upload session and deletes all temporary chunks.
func UploadAbort(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	fileID := strings.TrimSpace(c.DefaultPostForm("fileId", c.Query("fileId")))
	clientKey := strings.TrimSpace(c.DefaultPostForm("clientKey", c.Query("clientKey")))
	if fileID == "" && clientKey == "" {
		responseBody.Msg = "fileId or clientKey is required"
		return &responseBody
	}

	if fileID == "" && clientKey != "" {
		uploadStore.mu.RLock()
		fileID = uploadStore.fileIDByKey[clientKey]
		uploadStore.mu.RUnlock()
	}
	if fileID == "" {
		// Already cleaned or not found; keep idempotent behavior.
		return &responseBody
	}

	removeSession(fileID)
	_ = os.RemoveAll(resumableSessionDir(fileID))
	return &responseBody
}

// UploadStatus returns uploaded chunk indexes for a session.
func UploadStatus(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	fileID := strings.TrimSpace(c.Query("fileId"))
	if fileID == "" {
		responseBody.Msg = "fileId is required"
		return &responseBody
	}
	uploadStore.mu.RLock()
	session, ok := uploadStore.byID[fileID]
	if !ok {
		uploadStore.mu.RUnlock()
		responseBody.Msg = "upload session not found"
		return &responseBody
	}
	uploadedChunks := parseChunkList(session.Received)
	totalChunks := session.TotalChunks
	uploadStore.mu.RUnlock()
	responseBody.Data = gin.H{
		"fileId":        fileID,
		"uploadedChunks": uploadedChunks,
		"totalChunks":   totalChunks,
	}
	return &responseBody
}

// UploadChunk receives one file chunk.
func UploadChunk(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	fileID := strings.TrimSpace(c.PostForm("fileId"))
	chunkIndexStr := strings.TrimSpace(c.PostForm("chunkIndex"))
	if fileID == "" || chunkIndexStr == "" {
		responseBody.Msg = "fileId and chunkIndex are required"
		return &responseBody
	}
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		responseBody.Msg = "invalid chunkIndex"
		return &responseBody
	}
	uploadStore.mu.RLock()
	session, ok := uploadStore.byID[fileID]
	if !ok {
		uploadStore.mu.RUnlock()
		responseBody.Msg = "upload session not found"
		return &responseBody
	}
	totalChunks := session.TotalChunks
	uploadStore.mu.RUnlock()
	if chunkIndex >= totalChunks {
		responseBody.Msg = "chunkIndex out of range"
		return &responseBody
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer file.Close()

	chunkPath := resumableChunkPath(fileID, chunkIndex)
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0o755); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	dst, err := os.Create(chunkPath)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if _, err := io.Copy(dst, file); err != nil {
		_ = dst.Close()
		responseBody.Msg = err.Error()
		return &responseBody
	}
	_ = dst.Close()

	uploadStore.mu.Lock()
	if s, ok2 := uploadStore.byID[fileID]; ok2 {
		s.Received[chunkIndex] = struct{}{}
		s.UpdatedAt = time.Now()
	}
	uploadStore.mu.Unlock()

	responseBody.Data = gin.H{
		"fileId":      fileID,
		"chunkIndex":  chunkIndex,
	}
	return &responseBody
}

// UploadComplete merges all uploaded chunks and writes to remote SFTP target.
func UploadComplete(c *gin.Context) *ResponseBody {
	var (
		sshClient core.SSHClient
		err       error
	)
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	fileID := strings.TrimSpace(c.DefaultPostForm("fileId", c.Query("fileId")))
	if fileID == "" {
		responseBody.Msg = "fileId is required"
		return &responseBody
	}
	uploadStore.mu.RLock()
	session, ok := uploadStore.byID[fileID]
	if !ok {
		uploadStore.mu.RUnlock()
		responseBody.Msg = "upload session not found"
		return &responseBody
	}
	sessionCopy := *session
	receivedCount := len(session.Received)
	uploadedChunks := parseChunkList(session.Received)
	uploadStore.mu.RUnlock()

	if receivedCount != sessionCopy.TotalChunks {
		responseBody.Msg = "chunks not complete"
		responseBody.Data = gin.H{
			"uploadedChunks": uploadedChunks,
			"totalChunks":    sessionCopy.TotalChunks,
		}
		return &responseBody
	}

	if sshClient, err = core.DecodedMsgToSSHClient(sessionCopy.SSHInfo); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if err := sshClient.CreateSftp(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer sshClient.Close()

	path := strings.TrimSpace(sessionCopy.Path)
	if path == "" {
		path = detectHomeDir(sshClient.Sftp, sshClient.Username)
	}
	pathArr := []string{strings.TrimRight(path, "/")}
	if sessionCopy.Dir != "" {
		pathArr = append(pathArr, sessionCopy.Dir)
		if err := sshClient.Mkdirs(strings.Join(pathArr, "/")); err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
	}
	pathArr = append(pathArr, sessionCopy.FileName)
	remotePath := strings.Join(pathArr, "/")

	remoteFile, err := sshClient.Sftp.Create(remotePath)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer remoteFile.Close()

	for i := 0; i < sessionCopy.TotalChunks; i++ {
		chunkFile, err := os.Open(resumableChunkPath(fileID, i))
		if err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
		if _, err := io.Copy(remoteFile, chunkFile); err != nil {
			_ = chunkFile.Close()
			responseBody.Msg = err.Error()
			return &responseBody
		}
		_ = chunkFile.Close()
	}

	removeSession(fileID)
	_ = os.RemoveAll(resumableSessionDir(fileID))
	responseBody.Data = gin.H{
		"fileId": fileID,
		"path":   remotePath,
	}
	return &responseBody
}

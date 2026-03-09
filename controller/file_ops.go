package controller

import (
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"webssh/core"

	"github.com/gin-gonic/gin"
	"github.com/pkg/sftp"
)

func remoteBaseName(p string) string {
	clean := path.Clean(strings.TrimSpace(p))
	if clean == "." || clean == "/" {
		return ""
	}
	return path.Base(clean)
}

func removeRemoteRecursive(client *sftp.Client, targetPath string) error {
	info, err := client.Stat(targetPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return client.Remove(targetPath)
	}

	children, err := client.ReadDir(targetPath)
	if err != nil {
		return err
	}
	for _, child := range children {
		childPath := path.Join(targetPath, child.Name())
		if err := removeRemoteRecursive(client, childPath); err != nil {
			return err
		}
	}
	if err := client.RemoveDirectory(targetPath); err != nil {
		// Some SFTP servers only support Remove for directory deletion.
		return client.Remove(targetPath)
	}
	return nil
}

func copyRemoteFile(client *sftp.Client, srcPath, dstPath string) error {
	srcFile, err := client.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := client.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func copyRemoteRecursive(client *sftp.Client, srcPath, dstPath string) error {
	info, err := client.Stat(srcPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyRemoteFile(client, srcPath, dstPath)
	}

	if err := client.MkdirAll(dstPath); err != nil {
		return err
	}
	children, err := client.ReadDir(srcPath)
	if err != nil {
		return err
	}
	for _, child := range children {
		srcChild := path.Join(srcPath, child.Name())
		dstChild := path.Join(dstPath, child.Name())
		if err := copyRemoteRecursive(client, srcChild, dstChild); err != nil {
			return err
		}
	}
	return nil
}

// DeleteFileOrDir removes file or directory recursively.
func DeleteFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	targetPath := strings.TrimSpace(c.DefaultPostForm("path", ""))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if targetPath == "" || sshInfo == "" {
		responseBody.Msg = "path and sshInfo are required"
		return &responseBody
	}
	if path.Clean(targetPath) == "/" {
		responseBody.Msg = "refuse to delete root directory"
		return &responseBody
	}

	sshClient, err := core.DecodedMsgToSSHClient(sshInfo)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if err := sshClient.CreateSftp(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer sshClient.Close()

	if err := removeRemoteRecursive(sshClient.Sftp, targetPath); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	return &responseBody
}

// CopyFileOrDir validates copy source path exists.
func CopyFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	targetPath := strings.TrimSpace(c.DefaultPostForm("path", ""))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if targetPath == "" || sshInfo == "" {
		responseBody.Msg = "path and sshInfo are required"
		return &responseBody
	}

	sshClient, err := core.DecodedMsgToSSHClient(sshInfo)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if err := sshClient.CreateSftp(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer sshClient.Close()

	info, err := sshClient.Sftp.Stat(targetPath)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}

	responseBody.Data = gin.H{
		"path":  targetPath,
		"name":  remoteBaseName(targetPath),
		"isDir": info.IsDir(),
	}
	return &responseBody
}

// PasteFileOrDir copies src to dst directory.
func PasteFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	srcPath := strings.TrimSpace(c.DefaultPostForm("srcPath", ""))
	dstPath := strings.TrimSpace(c.DefaultPostForm("dstPath", ""))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if srcPath == "" || dstPath == "" || sshInfo == "" {
		responseBody.Msg = "srcPath, dstPath and sshInfo are required"
		return &responseBody
	}

	sshClient, err := core.DecodedMsgToSSHClient(sshInfo)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if err := sshClient.CreateSftp(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer sshClient.Close()

	srcInfo, err := sshClient.Sftp.Stat(srcPath)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}

	baseName := remoteBaseName(srcPath)
	if baseName == "" {
		responseBody.Msg = "invalid source path"
		return &responseBody
	}
	dstFullPath := path.Join(dstPath, baseName)
	if path.Clean(srcPath) == path.Clean(dstFullPath) {
		responseBody.Msg = "source and destination are the same"
		return &responseBody
	}

	if _, err := sshClient.Sftp.Stat(dstFullPath); err == nil {
		responseBody.Msg = fmt.Sprintf("target already exists: %s", dstFullPath)
		return &responseBody
	}

	if srcInfo.IsDir() {
		if err := copyRemoteRecursive(sshClient.Sftp, srcPath, dstFullPath); err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
	} else {
		if err := copyRemoteFile(sshClient.Sftp, srcPath, dstFullPath); err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
	}

	responseBody.Data = gin.H{
		"path": dstFullPath,
	}
	return &responseBody
}

// MoveFileOrDir moves src into dst directory.
func MoveFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	srcPath := strings.TrimSpace(c.DefaultPostForm("srcPath", ""))
	dstPath := strings.TrimSpace(c.DefaultPostForm("dstPath", ""))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if srcPath == "" || dstPath == "" || sshInfo == "" {
		responseBody.Msg = "srcPath, dstPath and sshInfo are required"
		return &responseBody
	}

	sshClient, err := core.DecodedMsgToSSHClient(sshInfo)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if err := sshClient.CreateSftp(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer sshClient.Close()

	baseName := remoteBaseName(srcPath)
	if baseName == "" {
		responseBody.Msg = "invalid source path"
		return &responseBody
	}
	dstFullPath := path.Join(dstPath, baseName)
	if path.Clean(srcPath) == path.Clean(dstFullPath) {
		responseBody.Msg = "source and destination are the same"
		return &responseBody
	}

	if _, err := sshClient.Sftp.Stat(dstFullPath); err == nil {
		responseBody.Msg = fmt.Sprintf("target already exists: %s", dstFullPath)
		return &responseBody
	}

	if err := sshClient.Sftp.Rename(srcPath, dstFullPath); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}

	responseBody.Data = gin.H{
		"path": dstFullPath,
	}
	return &responseBody
}

// RenameFileOrDir renames src name in the same directory.
func RenameFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	srcPath := strings.TrimSpace(c.DefaultPostForm("srcPath", ""))
	newName := strings.TrimSpace(c.DefaultPostForm("newName", ""))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if srcPath == "" || newName == "" || sshInfo == "" {
		responseBody.Msg = "srcPath, newName and sshInfo are required"
		return &responseBody
	}
	if strings.Contains(newName, "/") || strings.Contains(newName, "\\") {
		responseBody.Msg = "newName is invalid"
		return &responseBody
	}

	sshClient, err := core.DecodedMsgToSSHClient(sshInfo)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if err := sshClient.CreateSftp(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer sshClient.Close()

	srcDir := path.Dir(srcPath)
	if srcDir == "." {
		srcDir = "/"
	}
	dstPath := path.Join(srcDir, newName)
	if path.Clean(srcPath) == path.Clean(dstPath) {
		return &responseBody
	}
	if _, err := sshClient.Sftp.Stat(dstPath); err == nil {
		responseBody.Msg = fmt.Sprintf("target already exists: %s", dstPath)
		return &responseBody
	}
	if err := sshClient.Sftp.Rename(srcPath, dstPath); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	responseBody.Data = gin.H{
		"path": dstPath,
	}
	return &responseBody
}

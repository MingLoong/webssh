package controller

import (
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
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

func parseBoolFormValue(raw string) bool {
	v := strings.TrimSpace(strings.ToLower(raw))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func chmodRemoteRecursive(client *sftp.Client, targetPath string, mode os.FileMode) error {
	if err := client.Chmod(targetPath, mode); err != nil {
		return err
	}
	info, err := client.Stat(targetPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	children, err := client.ReadDir(targetPath)
	if err != nil {
		return err
	}
	for _, child := range children {
		childPath := path.Join(targetPath, child.Name())
		if err := chmodRemoteRecursive(client, childPath, mode); err != nil {
			return err
		}
	}
	return nil
}

func chownRemoteRecursive(client *sftp.Client, targetPath string, uid, gid int) error {
	if err := client.Chown(targetPath, uid, gid); err != nil {
		return err
	}
	info, err := client.Stat(targetPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	children, err := client.ReadDir(targetPath)
	if err != nil {
		return err
	}
	for _, child := range children {
		childPath := path.Join(targetPath, child.Name())
		if err := chownRemoteRecursive(client, childPath, uid, gid); err != nil {
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

// ChmodFileOrDir updates remote file/dir mode by octal string (e.g. 755/644).
func ChmodFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	targetPath := strings.TrimSpace(c.DefaultPostForm("path", ""))
	modeStr := strings.TrimSpace(c.DefaultPostForm("mode", ""))
	recursive := parseBoolFormValue(c.DefaultPostForm("recursive", "false"))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if targetPath == "" || modeStr == "" || sshInfo == "" {
		responseBody.Msg = "path, mode and sshInfo are required"
		return &responseBody
	}
	modeStr = strings.TrimPrefix(modeStr, "0")
	if modeStr == "" {
		modeStr = "0"
	}
	modeValue, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		responseBody.Msg = "invalid mode, use octal like 755"
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

	mode := os.FileMode(modeValue)
	var errChmod error
	if recursive {
		errChmod = chmodRemoteRecursive(sshClient.Sftp, targetPath, mode)
	} else {
		errChmod = sshClient.Sftp.Chmod(targetPath, mode)
	}
	if errChmod != nil {
		responseBody.Msg = errChmod.Error()
		return &responseBody
	}
	return &responseBody
}

// ListUserGroupCandidates returns remote user/group names for frontend autocomplete.
func ListUserGroupCandidates(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	sshInfo := strings.TrimSpace(c.DefaultQuery("sshInfo", ""))
	if sshInfo == "" {
		responseBody.Msg = "sshInfo is required"
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

	userMap, groupMap := loadRemoteUserGroupMaps(sshClient.Sftp)
	userSet := make(map[string]struct{}, len(userMap))
	groupSet := make(map[string]struct{}, len(groupMap))
	for _, user := range userMap {
		user = strings.TrimSpace(user)
		if user != "" {
			userSet[user] = struct{}{}
		}
	}
	for _, group := range groupMap {
		group = strings.TrimSpace(group)
		if group != "" {
			groupSet[group] = struct{}{}
		}
	}
	users := make([]string, 0, len(userSet))
	groups := make([]string, 0, len(groupSet))
	for user := range userSet {
		users = append(users, user)
	}
	for group := range groupSet {
		groups = append(groups, group)
	}
	sort.Strings(users)
	sort.Strings(groups)
	responseBody.Data = gin.H{
		"users":  users,
		"groups": groups,
	}
	return &responseBody
}

func reverseNameMap(idNameMap map[uint32]string) map[string]uint32 {
	result := make(map[string]uint32, len(idNameMap))
	for id, name := range idNameMap {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		result[name] = id
	}
	return result
}

func resolveUserOrGroupID(raw string, nameIDMap map[string]uint32, kind string) (uint32, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("%s is empty", kind)
	}
	if uid64, err := strconv.ParseUint(value, 10, 32); err == nil {
		return uint32(uid64), nil
	}
	if id, ok := nameIDMap[value]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("unknown %s: %s", kind, value)
}

// ChownFileOrDir updates file owner/group by user/group name or uid/gid.
func ChownFileOrDir(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	targetPath := strings.TrimSpace(c.DefaultPostForm("path", ""))
	ownerRaw := strings.TrimSpace(c.DefaultPostForm("owner", ""))
	groupRaw := strings.TrimSpace(c.DefaultPostForm("group", ""))
	recursive := parseBoolFormValue(c.DefaultPostForm("recursive", "false"))
	sshInfo := strings.TrimSpace(c.DefaultPostForm("sshInfo", ""))
	if targetPath == "" || sshInfo == "" {
		responseBody.Msg = "path and sshInfo are required"
		return &responseBody
	}
	if ownerRaw == "" && groupRaw == "" {
		responseBody.Msg = "owner or group is required"
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

	fileInfo, err := sshClient.Sftp.Stat(targetPath)
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	fileStat, ok := fileInfo.Sys().(*sftp.FileStat)
	if !ok {
		responseBody.Msg = "remote server does not support owner/group metadata"
		return &responseBody
	}
	uid := fileStat.UID
	gid := fileStat.GID

	userMap, groupMap := loadRemoteUserGroupMaps(sshClient.Sftp)
	nameUserMap := reverseNameMap(userMap)
	nameGroupMap := reverseNameMap(groupMap)

	if ownerRaw != "" {
		parsedUID, err := resolveUserOrGroupID(ownerRaw, nameUserMap, "owner")
		if err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
		uid = parsedUID
	}
	if groupRaw != "" {
		parsedGID, err := resolveUserOrGroupID(groupRaw, nameGroupMap, "group")
		if err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
		gid = parsedGID
	}

	var errChown error
	if recursive {
		errChown = chownRemoteRecursive(sshClient.Sftp, targetPath, int(uid), int(gid))
	} else {
		errChown = sshClient.Sftp.Chown(targetPath, int(uid), int(gid))
	}
	if errChown != nil {
		responseBody.Msg = errChown.Error()
		return &responseBody
	}
	return &responseBody
}

package core

import (
	"github.com/gorilla/websocket"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"sync"
	"unicode/utf8"
)

// WcList 全局counter list变量
var WcList []*WriteCounter
var wcMu sync.RWMutex

// WriteCounter 结构体
type WriteCounter struct {
	Total int
	Id    string
}

// AddWriteCounter appends a counter in a thread-safe way.
func AddWriteCounter(wc *WriteCounter) {
	if wc == nil {
		return
	}
	wcMu.Lock()
	WcList = append(WcList, wc)
	wcMu.Unlock()
}

// RemoveWriteCounterByID removes the counter by id in a thread-safe way.
func RemoveWriteCounterByID(id string) {
	wcMu.Lock()
	defer wcMu.Unlock()
	for i := 0; i < len(WcList); i++ {
		if WcList[i].Id == id {
			WcList = append(WcList[:i], WcList[i+1:]...)
			break
		}
	}
	if len(WcList) == 0 {
		WcList = nil
	}
}

// SnapshotWriteCounters returns a copy of current counters for safe iteration.
func SnapshotWriteCounters() []*WriteCounter {
	wcMu.RLock()
	defer wcMu.RUnlock()
	if len(WcList) == 0 {
		return nil
	}
	out := make([]*WriteCounter, len(WcList))
	copy(out, WcList)
	return out
}

// Write: implement Write interface to write bytes from ssh server into bytes.Buffer.
func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += n
	return n, nil
}

type wsOutput struct {
	ws *websocket.Conn
}

// Write: implement Write interface to write bytes from ssh server into bytes.Buffer.
func (w *wsOutput) Write(p []byte) (int, error) {
	// 处理非utf8字符
	if !utf8.Valid(p) {
		bufStr := string(p)
		buf := make([]rune, 0, len(bufStr))
		for _, r := range bufStr {
			if r == utf8.RuneError {
				buf = append(buf, []rune("@")...)
			} else {
				buf = append(buf, r)
			}
		}
		p = []byte(string(buf))
	}
	err := w.ws.WriteMessage(websocket.TextMessage, p)
	return len(p), err
}

// SSHClient 结构体
type SSHClient struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Hostname  string `json:"hostname"`
	Port      int    `json:"port"`
	LoginType int    `json:"logintype"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
	Client    *ssh.Client
	Sftp      *sftp.Client
	StdinPipe io.WriteCloser
	Session   *ssh.Session
}

// NewSSHClient 返回默认ssh信息
func NewSSHClient() SSHClient {
	client := SSHClient{}
	client.Port = 22
	return client
}

// Close all closable fields of SSHClient that is opened:
//
//	StdinPipe, Session, Sftp, Client
func (sclient *SSHClient) Close() {
	defer func() { // just in case
		if err := recover(); err != nil {
			log.Println("SSHClient Close recover from panic: ", err)
		}
	}()

	if sclient.StdinPipe != nil {
		sclient.StdinPipe.Close()
		sclient.StdinPipe = nil
	}
	if sclient.Session != nil {
		sclient.Session.Close()
		sclient.Session = nil
	}
	if sclient.Sftp != nil {
		sclient.Sftp.Close()
		sclient.Sftp = nil
	}
	if sclient.Client != nil {
		sclient.Client.Close()
		sclient.Client = nil
	}
}

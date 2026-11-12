package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	screenshotSourceADB    = "adb"
	screenshotSourceRemote = "remote"

	remoteScreenshotDeviceTimeout = 45 * time.Second
	remoteScreenshotTaskTimeout   = 60 * time.Second
	remoteScreenshotMaxImageBytes = 20 << 20
	remoteScreenshotMaxJSONBytes  = 28 << 20
)

var remoteScreenshotActive atomicBool

type atomicBool struct {
	mu    sync.RWMutex
	value bool
}

func (b *atomicBool) Load() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.value
}

func (b *atomicBool) Store(value bool) {
	b.mu.Lock()
	b.value = value
	b.mu.Unlock()
}

func screenshotSourceIsRemote() bool {
	return remoteScreenshotActive.Load()
}

func normalizeScreenshotSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case screenshotSourceRemote, "http", "remote_http", "远程", "远程http":
		return screenshotSourceRemote
	default:
		return screenshotSourceADB
	}
}

func screenshotSourceDisplay(source string) string {
	if normalizeScreenshotSource(source) == screenshotSourceRemote {
		return "远程 HTTP"
	}
	return "本地 ADB"
}

func screenshotSourceFromDisplay(display string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(display)), "remote") ||
		strings.Contains(strings.TrimSpace(display), "远程") {
		return screenshotSourceRemote
	}
	return screenshotSourceADB
}

type RemoteScreenshotStatus struct {
	Running    bool
	Port       int
	DeviceName string
	DeviceID   string
	LastSeen   time.Time
}

func (s RemoteScreenshotStatus) Online() bool {
	return s.Running && !s.LastSeen.IsZero() && time.Since(s.LastSeen) <= remoteScreenshotDeviceTimeout
}

type remoteScreenshotTask struct {
	ExpiresAt time.Time
	Result    chan remoteScreenshotResult
}

type remoteScreenshotResult struct {
	Image image.Image
	Err   error
}

type remoteScreenshotUpload struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    remoteScreenshotData `json:"data"`
}

type remoteScreenshotData struct {
	Base64 string `json:"base64"`
}

type remoteScreenshotCommand struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Task    string                 `json:"Task"`
	Data    map[string]interface{} `json:"data"`
}

type remoteScreenshotReply struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

type RemoteScreenshotServer struct {
	mu         sync.Mutex
	httpServer *http.Server
	listener   net.Listener
	port       int
	lastSeen   time.Time
	pending    *remoteScreenshotTask
	taskWake   chan struct{}
}

func NewRemoteScreenshotServer() *RemoteScreenshotServer {
	return &RemoteScreenshotServer{taskWake: make(chan struct{})}
}

func (s *RemoteScreenshotServer) Start(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("远程截图端口必须在 1 到 65535 之间")
	}

	s.mu.Lock()
	if s.httpServer != nil && s.port == port {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	s.Stop()
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return fmt.Errorf("监听远程截图端口 %d 失败: %w", port, err)
	}

	server := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       35 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	s.mu.Lock()
	s.listener = listener
	s.httpServer = server
	s.port = port
	s.lastSeen = time.Time{}
	s.pending = nil
	s.signalLocked()
	s.mu.Unlock()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[remote-screenshot] server stopped with error: %v", err)
		}
	}()
	log.Printf("[remote-screenshot] server started: listen=0.0.0.0:%d", port)
	return nil
}

func (s *RemoteScreenshotServer) Stop() {
	s.mu.Lock()
	server := s.httpServer
	pending := s.pending
	s.httpServer = nil
	s.listener = nil
	s.port = 0
	s.lastSeen = time.Time{}
	s.pending = nil
	s.signalLocked()
	s.mu.Unlock()

	if pending != nil {
		select {
		case pending.Result <- remoteScreenshotResult{Err: fmt.Errorf("远程截图服务已停止")}:
		default:
		}
	}
	if server != nil {
		_ = server.Close()
		log.Printf("[remote-screenshot] server stopped")
	}
}

func (s *RemoteScreenshotServer) Status() RemoteScreenshotStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return RemoteScreenshotStatus{
		Running:    s.httpServer != nil,
		Port:       s.port,
		DeviceName: "远程设备",
		DeviceID:   "remote-device",
		LastSeen:   s.lastSeen,
	}
}

func (s *RemoteScreenshotServer) RequestScreenshot() (image.Image, error) {
	s.mu.Lock()
	if s.httpServer == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("远程截图服务未启动")
	}
	if !s.deviceOnlineLocked() {
		s.mu.Unlock()
		return nil, fmt.Errorf("没有在线的远程截图设备")
	}
	if s.pending != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("已有一个远程截图任务正在处理")
	}
	task := &remoteScreenshotTask{
		ExpiresAt: time.Now().Add(remoteScreenshotTaskTimeout),
		Result:    make(chan remoteScreenshotResult, 1),
	}
	s.pending = task
	s.signalLocked()
	s.mu.Unlock()

	timer := time.NewTimer(remoteScreenshotTaskTimeout)
	defer timer.Stop()
	select {
	case result := <-task.Result:
		return result.Image, result.Err
	case <-timer.C:
		s.mu.Lock()
		if s.pending == task {
			s.pending = nil
			s.signalLocked()
		}
		s.mu.Unlock()
		return nil, fmt.Errorf("远程截图任务超时")
	}
}

func (s *RemoteScreenshotServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/remote/task":
		s.handleTaskPoll(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/remote/screenshot":
		s.handleScreenshotUpload(w, r)
	default:
		writeRemoteError(w, http.StatusNotFound, 404, "接口不存在")
	}
}

func (s *RemoteScreenshotServer) handleTaskPoll(w http.ResponseWriter, r *http.Request) {
	waitSeconds := parseRemoteWaitSeconds(r.URL.Query().Get("wait"))
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		s.mu.Lock()
		if s.httpServer == nil {
			s.mu.Unlock()
			writeRemoteError(w, http.StatusServiceUnavailable, 503, "远程截图服务未启动")
			return
		}
		s.lastSeen = time.Now()
		if s.pending != nil {
			if time.Now().After(s.pending.ExpiresAt) {
				expired := s.pending
				s.pending = nil
				s.signalLocked()
				s.mu.Unlock()
				select {
				case expired.Result <- remoteScreenshotResult{Err: fmt.Errorf("远程截图任务已过期")}:
				default:
				}
				continue
			}
			s.mu.Unlock()
			writeRemoteJSON(w, http.StatusOK, remoteScreenshotCommand{
				Code: 0, Message: "success", Task: "Screenshot", Data: map[string]interface{}{},
			})
			return
		}
		wake := s.taskWake
		s.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			writeRemoteJSON(w, http.StatusOK, remoteScreenshotCommand{
				Code: 0, Message: "success", Task: "", Data: map[string]interface{}{},
			})
			return
		}
		timer := time.NewTimer(remaining)
		select {
		case <-wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			writeRemoteJSON(w, http.StatusOK, remoteScreenshotCommand{
				Code: 0, Message: "success", Task: "", Data: map[string]interface{}{},
			})
			return
		case <-r.Context().Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
}

func (s *RemoteScreenshotServer) handleScreenshotUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, remoteScreenshotMaxJSONBytes)
	var upload remoteScreenshotUpload
	if err := json.NewDecoder(r.Body).Decode(&upload); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			writeRemoteError(w, http.StatusRequestEntityTooLarge, 413, "截图请求过大")
			return
		}
		writeRemoteError(w, http.StatusBadRequest, 400, "请求必须是有效的 JSON")
		return
	}

	encoded := normalizeRemoteScreenshotBase64(upload.Data.Base64)
	if encoded == "" {
		writeRemoteError(w, http.StatusBadRequest, 400, "截图内容为空")
		return
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		writeRemoteError(w, http.StatusBadRequest, 400, "base64 不是有效的图片数据")
		return
	}
	if len(data) == 0 {
		writeRemoteError(w, http.StatusBadRequest, 400, "截图内容为空")
		return
	}
	if len(data) > remoteScreenshotMaxImageBytes {
		writeRemoteError(w, http.StatusRequestEntityTooLarge, 413, "截图文件过大")
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		writeRemoteError(w, http.StatusBadRequest, 400, "base64 内容不是有效图片")
		return
	}

	s.mu.Lock()
	if s.pending == nil || time.Now().After(s.pending.ExpiresAt) {
		s.mu.Unlock()
		writeRemoteError(w, http.StatusConflict, 409, "当前没有等待中的截图任务")
		return
	}
	task := s.pending
	s.pending = nil
	s.lastSeen = time.Now()
	s.signalLocked()
	s.mu.Unlock()
	task.Result <- remoteScreenshotResult{Image: img}
	writeRemoteJSON(w, http.StatusOK, remoteScreenshotReply{
		Code: 0, Message: "success", Data: map[string]interface{}{},
	})
}

func (s *RemoteScreenshotServer) deviceOnlineLocked() bool {
	return !s.lastSeen.IsZero() && time.Since(s.lastSeen) <= remoteScreenshotDeviceTimeout
}

func (s *RemoteScreenshotServer) signalLocked() {
	if s.taskWake != nil {
		close(s.taskWake)
	}
	s.taskWake = make(chan struct{})
}

func parseRemoteWaitSeconds(value string) int {
	waitSeconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || waitSeconds <= 0 {
		return 20
	}
	if waitSeconds > 30 {
		return 30
	}
	return waitSeconds
}

func normalizeRemoteScreenshotBase64(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, "base64,"); idx >= 0 {
		value = value[idx+len("base64,"):]
	}
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\t", "")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func writeRemoteError(w http.ResponseWriter, status, code int, message string) {
	writeRemoteJSON(w, status, remoteScreenshotReply{
		Code: code, Message: message, Data: map[string]interface{}{},
	})
}

func writeRemoteJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

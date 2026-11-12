package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestRemoteScreenshotServerPollAndUpload(t *testing.T) {
	server := NewRemoteScreenshotServer()
	port := reserveRemoteScreenshotTestPort(t)
	if err := server.Start(port); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Stop()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	initialPoll := postRemoteJSON(t, baseURL+"/api/remote/task?wait=1", map[string]interface{}{})
	if initialPoll.Task != "" {
		t.Fatalf("initial poll returned task: %+v", initialPoll)
	}

	resultChannel := make(chan remoteScreenshotResult, 1)
	go func() {
		img, err := server.RequestScreenshot()
		resultChannel <- remoteScreenshotResult{Image: img, Err: err}
	}()

	command := postRemoteJSON(t, baseURL+"/api/remote/task?wait=2", map[string]interface{}{})
	if command.Code != 0 || command.Task != "Screenshot" {
		t.Fatalf("unexpected command: %+v", command)
	}

	var pngData bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(1, 0, color.RGBA{0, 255, 0, 255})
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	upload := remoteScreenshotUpload{
		Code:    0,
		Message: "success",
		Data: remoteScreenshotData{
			Base64: base64.StdEncoding.EncodeToString(pngData.Bytes()),
		},
	}
	responseBody := postRemoteJSONValue(t, baseURL+"/api/remote/screenshot", upload)
	var response remoteScreenshotReply
	if err := json.Unmarshal(responseBody, &response); err != nil {
		t.Fatalf("decode upload response: %v, body=%s", err, responseBody)
	}
	if response.Code != 0 || response.Message != "success" {
		t.Fatalf("unexpected upload response: %+v", response)
	}

	select {
	case result := <-resultChannel:
		if result.Err != nil {
			t.Fatalf("request screenshot: %v", result.Err)
		}
		if result.Image == nil || result.Image.Bounds().Dx() != 2 || result.Image.Bounds().Dy() != 1 {
			t.Fatalf("unexpected returned image: %v", result.Image)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request screenshot did not receive the uploaded image")
	}

	status := server.Status()
	if !status.Online() {
		t.Fatalf("remote device should be online: %+v", status)
	}
}

func TestRemoteScreenshotServerRejectsOfflineDevice(t *testing.T) {
	server := NewRemoteScreenshotServer()
	port := reserveRemoteScreenshotTestPort(t)
	if err := server.Start(port); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Stop()

	server.mu.Lock()
	server.lastSeen = time.Now().Add(-remoteScreenshotDeviceTimeout - time.Second)
	server.mu.Unlock()
	if _, err := server.RequestScreenshot(); err == nil {
		t.Fatal("expected offline device to reject screenshot request")
	}
}

func postRemoteJSON(t *testing.T, url string, value interface{}) remoteScreenshotCommand {
	t.Helper()
	response := postRemoteJSONValue(t, url, value)
	var command remoteScreenshotCommand
	if err := json.Unmarshal(response, &command); err != nil {
		t.Fatalf("decode command: %v, body=%s", err, response)
	}
	return command
}

func postRemoteJSONValue(t *testing.T, url string, value interface{}) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	defer response.Body.Close()
	var responseBody bytes.Buffer
	if _, err := responseBody.ReadFrom(response.Body); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.StatusCode, responseBody.String())
	}
	return responseBody.Bytes()
}

func reserveRemoteScreenshotTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

//go:build windows && amd64 && !opencv_cgo

package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

//go:embed third_party/opencv/windows-amd64/bin/*.dll
var embeddedOpenCVLibraries embed.FS

var embeddedOpenCVBackend struct {
	sync.Once
	proc uintptr
	err  error
}

type embeddedOpenCVRequest struct {
	source, template                              *byte
	sourceWidth, sourceHeight, sourceStride       int32
	templateWidth, templateHeight, templateStride int32
	x1, y1, x2, y2                                int32
	isGray, isTransparent                         int32
	sim                                           float32
	findAll                                       int32
}

type embeddedOpenCVPoint struct {
	x, y  int32
	score float32
}

type embeddedOpenCVResult struct {
	count     int32
	bestScore float32
	errorCode int32
	message   [256]byte
}

func openCVImageMatchBackendName() string { return "内置 OpenCV" }

func openCVImageMatchBackendAvailable() bool {
	return loadEmbeddedOpenCVBackend() == nil
}

func loadEmbeddedOpenCVBackend() error {
	embeddedOpenCVBackend.Do(func() {
		cacheRoot, err := os.UserCacheDir()
		if err != nil {
			embeddedOpenCVBackend.err = err
			return
		}
		dir, err := extractEmbeddedOpenCVLibraries(cacheRoot)
		if err != nil {
			embeddedOpenCVBackend.err = err
			return
		}
		path, err := syscall.UTF16PtrFromString(filepath.Join(dir, "autogo_opencv_bridge.dll"))
		if err != nil {
			embeddedOpenCVBackend.err = err
			return
		}
		// Resolve dependencies from the extracted DLL directory and Windows system directories.
		loadLibrary := syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryExW")
		handle, _, loadErr := loadLibrary.Call(uintptr(unsafe.Pointer(path)), 0, 0x00000100|0x00001000)
		runtime.KeepAlive(path)
		if handle == 0 {
			embeddedOpenCVBackend.err = fmt.Errorf("加载内置 OpenCV 失败: %w", loadErr)
			return
		}
		proc, err := syscall.GetProcAddress(syscall.Handle(handle), "autogo_match_template_request")
		if err != nil {
			_ = syscall.FreeLibrary(syscall.Handle(handle))
			embeddedOpenCVBackend.err = err
			return
		}
		// Keep the library loaded for the process lifetime, including concurrent matches.
		embeddedOpenCVBackend.proc = proc
	})
	return embeddedOpenCVBackend.err
}

func extractEmbeddedOpenCVLibraries(cacheRoot string) (string, error) {
	const sourceDir = "third_party/opencv/windows-amd64/bin"
	entries, err := embeddedOpenCVLibraries.ReadDir(sourceDir)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, entry := range entries {
		data, err := embeddedOpenCVLibraries.ReadFile(sourceDir + "/" + entry.Name())
		if err != nil {
			return "", err
		}
		digest.Write([]byte(entry.Name()))
		digest.Write(data)
	}
	dir := filepath.Join(cacheRoot, "AutoGo", "opencv", fmt.Sprintf("%x", digest.Sum(nil)))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("创建 OpenCV 缓存目录失败: %w", err)
	}
	for _, entry := range entries {
		data, err := embeddedOpenCVLibraries.ReadFile(sourceDir + "/" + entry.Name())
		if err != nil {
			return "", err
		}
		if err := writeEmbeddedOpenCVLibrary(filepath.Join(dir, entry.Name()), data); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func writeEmbeddedOpenCVLibrary(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".opencv-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		// Another process may have extracted the same version at the same time.
		if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) {
			return nil
		}
		return err
	}
	return nil
}

func runOpenCVImageMatchBackend(source, template *image.NRGBA, x1, y1, x2, y2 int, opts openCVMatchOptions) (openCVMatchResult, error) {
	result := openCVMatchResult{Backend: openCVImageMatchBackendName()}
	if source == nil || template == nil || len(source.Pix) == 0 || len(template.Pix) == 0 {
		return result, fmt.Errorf("OpenCV 输入图片为空")
	}
	if err := loadEmbeddedOpenCVBackend(); err != nil {
		return result, fmt.Errorf("内置 OpenCV 不可用: %w", err)
	}
	maxResults := opts.MaxResults
	if !opts.FindAll || maxResults <= 0 {
		maxResults = 1
	}
	request := embeddedOpenCVRequest{
		source: &source.Pix[0], template: &template.Pix[0],
		sourceWidth: int32(source.Bounds().Dx()), sourceHeight: int32(source.Bounds().Dy()), sourceStride: int32(source.Stride),
		templateWidth: int32(template.Bounds().Dx()), templateHeight: int32(template.Bounds().Dy()), templateStride: int32(template.Stride),
		x1: int32(x1), y1: int32(y1), x2: int32(x2), y2: int32(y2), sim: opts.Sim,
	}
	if opts.IsGray {
		request.isGray = 1
	}
	if opts.IsTransparent {
		request.isTransparent = 1
	}
	if opts.FindAll {
		request.findAll = 1
	}
	points := make([]embeddedOpenCVPoint, maxResults)
	var nativeResult embeddedOpenCVResult
	syscall.SyscallN(embeddedOpenCVBackend.proc,
		uintptr(unsafe.Pointer(&request)), uintptr(unsafe.Pointer(&points[0])),
		uintptr(maxResults), uintptr(unsafe.Pointer(&nativeResult)))
	runtime.KeepAlive(source)
	runtime.KeepAlive(template)
	runtime.KeepAlive(request)
	result.BestScore = nativeResult.bestScore
	if nativeResult.errorCode != 0 {
		message := nativeResult.message[:]
		if end := bytes.IndexByte(message, 0); end >= 0 {
			message = message[:end]
		}
		return result, fmt.Errorf("%s", message)
	}
	if nativeResult.count < 0 || int(nativeResult.count) > len(points) {
		return result, fmt.Errorf("OpenCV 返回了无效的匹配数量: %d", nativeResult.count)
	}
	for _, point := range points[:int(nativeResult.count)] {
		result.Matches = append(result.Matches, openCVMatch{Point: image.Pt(int(point.x), int(point.y)), Score: point.score})
	}
	return result, nil
}

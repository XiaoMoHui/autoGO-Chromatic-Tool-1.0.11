package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultScreenshotSavePath = "screenshots"

func saveScreenshotToDirectory(img image.Image, directory string) (string, error) {
	if img == nil {
		return "", fmt.Errorf("截图为空")
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", fmt.Errorf("自动保存目录为空")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("创建自动保存目录失败: %w", err)
	}

	filePath := filepath.Join(directory, "screenshot_"+time.Now().Format("20060102_150405_000000000")+".png")
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("创建自动保存文件失败: %w", err)
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		_ = os.Remove(filePath)
		return "", fmt.Errorf("编码自动保存图片失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("关闭自动保存文件失败: %w", err)
	}
	return filePath, nil
}

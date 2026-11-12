package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveScreenshotToDirectory(t *testing.T) {
	directory := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(1, 0, color.RGBA{0, 255, 0, 255})

	filePath, err := saveScreenshotToDirectory(img, filepath.Join(directory, "screenshots"))
	if err != nil {
		t.Fatalf("save screenshot: %v", err)
	}
	if filepath.Ext(filePath) != ".png" {
		t.Fatalf("unexpected screenshot extension: %s", filePath)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("saved screenshot does not exist: %v", err)
	}
}

func TestSaveScreenshotToDirectoryRejectsEmptyPath(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if _, err := saveScreenshotToDirectory(img, ""); err == nil {
		t.Fatal("expected empty path to be rejected")
	}
}

//go:build windows && amd64 && !opencv_cgo

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestEmbeddedOpenCVNativeLayout(t *testing.T) {
	if unsafe.Sizeof(embeddedOpenCVRequest{}) != 72 || unsafe.Sizeof(embeddedOpenCVPoint{}) != 12 || unsafe.Sizeof(embeddedOpenCVResult{}) != 268 {
		t.Fatal("embedded OpenCV structures do not match the native bridge ABI")
	}
}

func TestEmbeddedOpenCVExtractionCanBeReused(t *testing.T) {
	cache := t.TempDir()
	dir, err := extractEmbeddedOpenCVLibraries(cache)
	if err != nil {
		t.Fatal(err)
	}
	again, err := extractEmbeddedOpenCVLibraries(cache)
	if err != nil {
		t.Fatal(err)
	}
	if again != dir {
		t.Fatal("unchanged embedded libraries should reuse their cache")
	}
	entries, err := embeddedOpenCVLibraries.ReadDir("third_party/opencv/windows-amd64/bin")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		want, err := embeddedOpenCVLibraries.ReadFile("third_party/opencv/windows-amd64/bin/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("extracted library differs: %s", entry.Name())
		}
	}
}

func TestEmbeddedOpenCVRunsOutsideSourceTree(t *testing.T) {
	if os.Getenv("AUTOGO_OPENCV_ISOLATED_TEST") == "1" {
		TestOpenCVBackendFindsKnownTemplate(t)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	isolatedDir := t.TempDir()
	cmd := exec.Command(executable, "-test.run=^TestEmbeddedOpenCVRunsOutsideSourceTree$", "-test.v")
	cmd.Dir = isolatedDir
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if strings.EqualFold(key, "PATH") || strings.EqualFold(key, "LOCALAPPDATA") || key == "AUTOGO_OPENCV_ISOLATED_TEST" {
			continue
		}
		cmd.Env = append(cmd.Env, variable)
	}
	cmd.Env = append(cmd.Env,
		"AUTOGO_OPENCV_ISOLATED_TEST=1",
		"LOCALAPPDATA="+filepath.Join(isolatedDir, "运行缓存"),
		"PATH="+filepath.Join(os.Getenv("SystemRoot"), "System32"),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("embedded backend failed without source directory or OpenCV PATH: %v\n%s", err, output)
	}
}

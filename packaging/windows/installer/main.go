package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var payloadMarker = []byte("WPS_READ_ALOUD_COMATE_PAYLOAD_ZIP_V1\n")

func main() {
	if err := run(); err != nil {
		showMessage("WPS 文档朗读助手安装失败", friendlyInstallError(err), 0x10)
		os.Exit(1)
	}
	showMessage("WPS 文档朗读助手", "安装完成。若 WPS 已打开，请重启 WPS。", 0x40)
}

func run() error {
	payload, err := readPayload()
	if err != nil {
		return err
	}
	tempRoot, err := os.MkdirTemp("", "wps-read-aloud-comate-installer-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)
	if err := extractZip(payload, tempRoot); err != nil {
		return err
	}
	installer := filepath.Join(tempRoot, "install.ps1")
	if _, err := os.Stat(installer); err != nil {
		return fmt.Errorf("安装包不完整，未找到 install.ps1")
	}
	cmd := exec.Command(
		powershellPath(),
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		installer,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Dir = tempRoot
	return cmd.Run()
}

func powershellPath() string {
	windir := os.Getenv("WINDIR")
	candidates := []string{}
	if windir != "" {
		candidates = append(candidates,
			filepath.Join(windir, "Sysnative", "WindowsPowerShell", "v1.0", "powershell.exe"),
			filepath.Join(windir, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
		)
	}
	candidates = append(candidates, "powershell.exe")
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			continue
		}
		if found, err := exec.LookPath(candidate); err == nil {
			return found
		}
	}
	return "powershell.exe"
}

func showMessage(title, text string, flags uintptr) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	messageBox.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))),
		flags,
	)
}

func friendlyInstallError(err error) string {
	logPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "WPSReadAloudComate", "Logs", "install.log")
	message := "安装没有完成。\n\n请查看安装日志：\n" + logPath
	if err != nil {
		message += "\n\n错误代码：" + err.Error()
	}
	return message
}

func readPayload() ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return nil, err
	}
	offset := bytes.LastIndex(data, payloadMarker)
	if offset < 0 {
		return nil, fmt.Errorf("安装程序缺少内嵌 payload")
	}
	payload := data[offset+len(payloadMarker):]
	if len(payload) == 0 {
		return nil, fmt.Errorf("安装程序内嵌 payload 为空")
	}
	return payload, nil
}

func extractZip(data []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		target := filepath.Join(dest, file.Name)
		cleanDest, err := filepath.Abs(dest)
		if err != nil {
			return err
		}
		cleanTarget, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if cleanTarget != cleanDest && !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("安装包包含非法路径：%s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

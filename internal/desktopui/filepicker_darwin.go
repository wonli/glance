//go:build darwin

package desktopui

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func chooseOfficeFile() (string, error) {
	script := `set pickedFile to choose file with prompt "选择 Office、JSON 或 Markdown 文档" of type {"xlsx", "xls", "docx", "json", "md", "markdown"}
POSIX path of pickedFile`
	cmd := exec.Command("osascript", "-e", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if strings.Contains(message, "User canceled") {
			return "", nil
		}
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseOfficeFolder() (string, error) {
	cmd := exec.Command("osascript", "-e", `set pickedFolder to choose folder with prompt "选择文件夹"`+"\n"+`POSIX path of pickedFolder`)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "User canceled") {
			return "", nil
		}
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func chooseSavePath(ext string) (string, error) {
	cmd := exec.Command("osascript", "-e", fmt.Sprintf(`set pickedFile to choose file name with prompt "另存为" default name "未命名.%s"`+"\n"+`POSIX path of pickedFile`, ext))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "User canceled") {
			return "", nil
		}
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func chooseOfficeImageFile() (string, error) {
	cmd := exec.Command("osascript", "-e", `set pickedFile to choose file with prompt "选择 PNG 或 JPEG 图片"`+"\n"+`POSIX path of pickedFile`)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if strings.Contains(message, "User canceled") {
			return "", nil
		}
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return strings.TrimSpace(string(output)), nil
}

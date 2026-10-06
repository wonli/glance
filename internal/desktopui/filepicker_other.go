//go:build !darwin && !windows

package desktopui

import (
	"fmt"
	"os/exec"
	"strings"
)

func chooseOfficeFile() (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", fmt.Errorf("请安装 zenity，或在路径框中输入文档路径")
	}
	output, err := exec.Command("zenity", "--file-selection", "--file-filter=Office/JSON | *.xlsx *.xls *.docx *.json *.md *.markdown").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseOfficeFolder() (string, error) {
	output, err := exec.Command("zenity", "--file-selection", "--directory").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseSavePath(ext string) (string, error) {
	output, err := exec.Command("zenity", "--file-selection", "--save", "--confirm-overwrite", "--filename=未命名."+ext, "--file-filter=Document | *."+ext).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseOfficeImageFile() (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", fmt.Errorf("请安装 zenity，或在文件选择器中选择 PNG/JPEG 图片")
	}
	output, err := exec.Command("zenity", "--file-selection", "--file-filter=Images | *.png *.jpg *.jpeg").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

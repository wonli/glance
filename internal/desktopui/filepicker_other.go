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
	output, err := exec.Command("zenity", "--file-selection", "--file-filter=Office documents | *.xlsx *.docx").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

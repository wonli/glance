//go:build darwin

package desktopui

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

func chooseOfficeFile() (string, error) {
	script := `set pickedFile to choose file with prompt "选择 Excel 或 Word 文档" of type {"xlsx", "docx"}
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

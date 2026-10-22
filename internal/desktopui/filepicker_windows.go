//go:build windows

package desktopui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func chooseOfficeFile(labels filePickerLabels) (string, error) {
	command := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Title = '" + powerShellString(labels.OpenDocument) + "'; $d.Filter = '" + powerShellString(labels.windowsOfficeFileFilter()) + "'; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.FileName }"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("open file picker: %s", message)
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseOfficeFolder(labels filePickerLabels) (string, error) {
	return chooseOfficeFolderContext(context.Background(), labels)
}

func chooseOfficeFolderContext(ctx context.Context, labels filePickerLabels) (string, error) {
	command := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = '" + powerShellString(labels.OpenFolder) + "'; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.SelectedPath }"
	return windowsPickerContext(ctx, command)
}

func chooseSavePath(ext string, labels filePickerLabels) (string, error) {
	command := fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.SaveFileDialog; $d.Title = '%s'; $d.Filter = '%s (*.%s)|*.%s'; $d.FileName = '%s.%s'; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.FileName }", powerShellString(labels.SaveAs), powerShellString(labels.SaveFilter), ext, ext, powerShellString(labels.UntitledFile), ext)
	return windowsPicker(command)
}

func windowsPicker(command string) (string, error) {
	return windowsPickerContext(context.Background(), command)
}

func windowsPickerContext(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseOfficeImageFile(labels filePickerLabels) (string, error) {
	command := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Title = '" + powerShellString(labels.ImagePrompt) + "'; $d.Filter = '" + powerShellString(labels.ImageFilter) + " (*.png;*.jpg;*.jpeg)|*.png;*.jpg;*.jpeg'; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.FileName }"
	return windowsPicker(command)
}

func choosePresentationMediaFile(kind string, labels filePickerLabels) (string, error) {
	filter := powerShellString(labels.AudioFilter) + " (*.mp3;*.wav)|*.mp3;*.wav"
	title := labels.AudioPrompt
	if kind == "video" {
		filter = powerShellString(labels.VideoFilter) + " (*.mp4;*.webm)|*.mp4;*.webm"
		title = labels.VideoPrompt
	}
	command := fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Title = '%s'; $d.Filter = '%s'; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.FileName }", powerShellString(title), filter)
	return windowsPicker(command)
}

func powerShellString(value string) string { return strings.ReplaceAll(value, "'", "''") }

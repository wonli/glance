//go:build darwin

package desktopui

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

func chooseOfficeFile(labels filePickerLabels) (string, error) {
	types := make([]string, 0, len(labels.officeFileExtensions()))
	for _, extension := range labels.officeFileExtensions() {
		types = append(types, appleScriptString(strings.TrimPrefix(extension, ".")))
	}
	script := `set pickedFile to choose file with prompt ` + appleScriptString(labels.OpenDocument) + ` of type {` + strings.Join(types, ", ") + `}
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

func chooseOfficeFolder(labels filePickerLabels) (string, error) {
	return chooseOfficeFolderContext(context.Background(), labels)
}

func chooseOfficeFolderContext(ctx context.Context, labels filePickerLabels) (string, error) {
	cmd := exec.CommandContext(ctx, "osascript", "-e", `set pickedFolder to choose folder with prompt `+appleScriptString(labels.OpenFolder)+"\n"+`POSIX path of pickedFolder`)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if strings.Contains(stderr.String(), "User canceled") {
			return "", nil
		}
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func chooseSavePath(ext string, labels filePickerLabels) (string, error) {
	cmd := exec.Command("osascript", "-e", `set pickedFile to choose file name with prompt `+appleScriptString(labels.SaveAs)+` default name `+appleScriptString(labels.UntitledFile+"."+ext)+"\n"+`POSIX path of pickedFile`)
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

func chooseOfficeImageFile(labels filePickerLabels) (string, error) {
	cmd := exec.Command("osascript", "-e", `set pickedFile to choose file with prompt `+appleScriptString(labels.ImagePrompt)+"\n"+`POSIX path of pickedFile`)
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

func choosePresentationMediaFile(kind string, labels filePickerLabels) (string, error) {
	prompt, types := labels.AudioPrompt, `{"mp3", "wav"}`
	if kind == "video" {
		prompt, types = labels.VideoPrompt, `{"mp4", "webm"}`
	}
	script := "set pickedFile to choose file with prompt " + appleScriptString(prompt) + " of type " + types + "\nPOSIX path of pickedFile"
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

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

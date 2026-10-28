//go:build !darwin && !windows

package desktopui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func chooseOfficeFile(labels filePickerLabels) (string, error) {
	return chooseOfficeFileContext(context.Background(), labels)
}

func chooseOfficeFileContext(ctx context.Context, labels filePickerLabels) (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", fmt.Errorf("%s", labels.InstallPickerNotice)
	}
	output, err := exec.CommandContext(ctx, "zenity", "--file-selection", "--title="+labels.OpenDocument, "--file-filter="+labels.DocumentFilter+" | "+labels.officeFileGlobs(" ")).Output()
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

func chooseOfficeFilesContext(ctx context.Context, labels filePickerLabels) ([]string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return nil, fmt.Errorf("%s", labels.InstallPickerNotice)
	}
	output, err := exec.CommandContext(ctx, "zenity", "--file-selection", "--multiple", "--separator=\n", "--title="+labels.OpenDocument, "--file-filter="+labels.DocumentFilter+" | "+labels.officeFileGlobs(" ")).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, err
	}
	return splitFilePickerPaths(string(output)), nil
}

func chooseOfficeFolder(labels filePickerLabels) (string, error) {
	return chooseOfficeFolderContext(context.Background(), labels)
}

func chooseOfficeFolderContext(ctx context.Context, labels filePickerLabels) (string, error) {
	output, err := exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title="+labels.OpenFolder).Output()
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

func chooseSavePath(ext string, labels filePickerLabels) (string, error) {
	output, err := exec.Command("zenity", "--file-selection", "--save", "--confirm-overwrite", "--title="+labels.SaveAs, "--filename="+labels.UntitledFile+"."+ext, "--file-filter="+labels.SaveFilter+" | *."+ext).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseOfficeImageFile(labels filePickerLabels) (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", fmt.Errorf("%s", labels.ImagePickerNotice)
	}
	output, err := exec.Command("zenity", "--file-selection", "--title="+labels.ImagePrompt, "--file-filter="+labels.ImageFilter+" | *.png *.jpg *.jpeg").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func choosePresentationMediaFile(kind string, labels filePickerLabels) (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", fmt.Errorf("%s", labels.MediaPickerNotice)
	}
	title, filter := labels.AudioPrompt, labels.AudioFilter+" | *.mp3 *.wav"
	if kind == "video" {
		title, filter = labels.VideoPrompt, labels.VideoFilter+" | *.mp4 *.webm"
	}
	output, err := exec.Command("zenity", "--file-selection", "--title="+title, "--file-filter="+filter).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

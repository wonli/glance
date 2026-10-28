package desktopui

import (
	"strings"
)

type filePickerLabels struct {
	OpenDocument        string
	DocumentFilter      string
	AllowedExtensions   []string
	OpenFolder          string
	SaveAs              string
	UntitledFile        string
	SaveFilter          string
	ImagePrompt         string
	ImageFilter         string
	AudioPrompt         string
	AudioFilter         string
	VideoPrompt         string
	VideoFilter         string
	InstallPickerNotice string
	ImagePickerNotice   string
	MediaPickerNotice   string
}

func (l filePickerLabels) officeFileExtensions() []string {
	if len(l.AllowedExtensions) > 0 {
		return l.AllowedExtensions
	}
	return []string{"xlsx", "xls", "doc", "docx", "json", "md", "markdown", "pdf", "csv", "txt"}
}

func (l filePickerLabels) officeFileGlobs(separator string) string {
	patterns := make([]string, 0, len(l.officeFileExtensions()))
	for _, extension := range l.officeFileExtensions() {
		patterns = append(patterns, "*."+strings.TrimPrefix(extension, "."))
	}
	return strings.Join(patterns, separator)
}

func (l filePickerLabels) windowsOfficeFileFilter() string {
	patterns := l.officeFileGlobs(";")
	return l.DocumentFilter + " (" + patterns + ")|" + patterns
}

func splitFilePickerPaths(output string) []string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.TrimRight(output, "\r\n")
	if output == "" {
		return nil
	}
	delimiter := "\n"
	if strings.Contains(output, "\x00") {
		delimiter = "\x00"
	}
	parts := strings.Split(output, delimiter)
	paths := make([]string, 0, len(parts))
	for _, path := range parts {
		path = strings.TrimSuffix(path, "\r")
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func (a *App) filePickerLabels() filePickerLabels {
	return filePickerLabels{
		OpenDocument:        a.textID("filepicker.open_document"),
		DocumentFilter:      a.textID("filepicker.document_filter"),
		OpenFolder:          a.textID("filepicker.open_folder"),
		SaveAs:              a.textID("filepicker.save_as"),
		UntitledFile:        a.textID("filepicker.untitled_file"),
		SaveFilter:          a.textID("filepicker.save_filter"),
		ImagePrompt:         a.textID("filepicker.image_prompt"),
		ImageFilter:         a.textID("filepicker.image_filter"),
		AudioPrompt:         a.textID("filepicker.audio_prompt"),
		AudioFilter:         a.textID("filepicker.audio_filter"),
		VideoPrompt:         a.textID("filepicker.video_prompt"),
		VideoFilter:         a.textID("filepicker.video_filter"),
		InstallPickerNotice: a.textID("filepicker.install_picker_notice"),
		ImagePickerNotice:   a.textID("filepicker.image_picker_notice"),
		MediaPickerNotice:   a.textID("filepicker.media_picker_notice"),
	}
}

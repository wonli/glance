package desktopui

type filePickerLabels struct {
	OpenDocument        string
	DocumentFilter      string
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

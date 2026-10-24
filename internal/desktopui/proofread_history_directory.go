package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"os/exec"
	"runtime"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
)

func (a *App) layoutProofreadingHistoryOpenGitDirectory(gtx layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	button := w.button("history-open-git-directory")
	enabled := w.historyProject.Directory != "" && !a.fileCommandRunning
	if button.Clicked(gtx) && enabled {
		a.startFileCommand(commandOpenDirectory, w.historyProject.Directory)
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	size := image.Pt(gtx.Dp(259), gtx.Dp(40))
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(5)).Op(gtx.Ops))
		proofreadRoundedBorder(gtx, size, 5, color.NRGBA{R: 204, G: 215, B: 223, A: 255}, max(1, gtx.Dp(1)))
		ink := color.NRGBA{R: 67, G: 86, B: 101, A: 255}
		if !enabled {
			ink = proofreadMuted
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutProofreadingIcon(gtx, "home", "folder-open", 16, ink)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Label(a.theme, 12, a.textID("proofread.history.open_git_directory"))
						label.Color = ink
						label.MaxLines = 1
						return layoutProofreadingOpticallyCenteredLabel(gtx, label)
					})
				}),
			)
		})
	})
}

// openLocalDirectory runs only on the managed file-command worker, never in a
// Gio frame. Open the task repository root (which contains pages/ and .git/).
func openLocalDirectory(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("文件夹路径为空")
	}
	target, err := validateHomeFolderDirectory(path)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", "--", target)
	case "windows":
		command = exec.Command("explorer.exe", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("打开文件夹失败：%w %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

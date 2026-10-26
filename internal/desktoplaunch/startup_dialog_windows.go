//go:build windows

package desktoplaunch

import (
	"fmt"

	"glance/internal/fileutil"

	"golang.org/x/sys/windows"
)

func showStartupError(err error) {
	if err == nil {
		return
	}
	message, messageErr := windows.UTF16PtrFromString(fmt.Sprintf("Glance 无法正常启动。\r\n\r\n%s\r\n\r\n启动日志位于用户配置目录下的 %s/startup.log。", err, fileutil.AppDir))
	if messageErr != nil {
		return
	}
	caption, captionErr := windows.UTF16PtrFromString("Glance 启动错误")
	if captionErr != nil {
		return
	}
	_, _ = windows.MessageBox(0, message, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}

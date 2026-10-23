APP_NAME = glance
APP_PATH = ./cmd/glance

# 编译路径
BUILD_PATH := ./dist

# 编译时间
BUILD_DATE = $(shell date +'%F %T')

GIT_BRANCH = -
GIT_COMMIT = -
GIT_REVISION = -

ifneq ($(wildcard .git),)
GIT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
GIT_COMMIT = $(shell git rev-list --count HEAD)
GIT_REVISION = $(shell git rev-parse --short HEAD)
endif

# support包名称
FLAGS_PKG = github.com/wonli/aqi

LDFLAGS = -X '$(FLAGS_PKG).BuildDate=$(BUILD_DATE)' \
		   -X '$(FLAGS_PKG).Branch=$(GIT_BRANCH)' \
		   -X '$(FLAGS_PKG).CommitVersion=$(GIT_COMMIT)' \
		   -X '$(FLAGS_PKG).Revision=$(GIT_REVISION)' \
		   -s -w

# 编译参数
GO_FLAGS = -ldflags "$(LDFLAGS)" -trimpath -tags netgo

# Go编译命令 1-GOOS 2-GOARCH 3-FILE EXT
define go/build
	go mod tidy
	GOOS=$(1) GOARCH=$(2) CGO_ENABLED=0 go build $(GO_FLAGS) -o $(BUILD_PATH)/$(APP_NAME)-$(1)-$(2)-latest$(3) ${APP_PATH}
endef

# Apple ld warns because Gio and the native print adapter each link Objective-C.
DESKTOP_GOOS ?= $(shell go env GOOS)
DESKTOP_LINK_FLAGS =
ifeq ($(DESKTOP_GOOS),darwin)
DESKTOP_LINK_FLAGS = -ldflags=-extldflags=-Wl,-no_warn_duplicate_libraries
endif

# Generate binaries
.PHONY: build darwin linux windows
.PHONY: desktop run-desktop

desktop:
	mkdir -p $(BUILD_PATH)
	go build $(DESKTOP_LINK_FLAGS) -o $(BUILD_PATH)/glance-office ${APP_PATH}

run-desktop:
	go run $(DESKTOP_LINK_FLAGS) ${APP_PATH} $(ARGS)

darwin:
	$(call go/build,darwin,arm64)

linux:
	$(call go/build,linux,amd64)

windows:
	mkdir -p $(BUILD_PATH)
	$(MAKE) brand-assets
	@set -eu; \
	icon_file="$(WINDOWS_ICON_ICO)"; \
	exe_file="$(BUILD_PATH)/$(APP_NAME)-windows-amd64-latest.exe"; \
	trap 'rm -f "$$icon_file"' EXIT; \
	go run ./packaging/windows/iconbuilder "$(BRAND_ASSETS_DIR)/glance-dock-icon.png" "$$icon_file"; \
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS) -H=windowsgui" -trimpath -tags netgo -o "$$exe_file" .; \
	go run ./packaging/windows/embedicon "$$exe_file" "$$icon_file"

MACOS_ARCH ?= $(shell go env GOARCH)
MACOS_APP_OUTPUT ?= $(BUILD_PATH)/Glance.app
MACOS_LINKER_FLAGS = $(LDFLAGS) -extldflags=-Wl,-no_warn_duplicate_libraries
MACOS_ICONSET = $(BUILD_PATH)/.macos-iconsets/$(MACOS_ARCH).iconset
MACOS_DOCK_ICON = internal/desktopui/assets/brand/glance-dock-icon.png
BRAND_ASSETS_DIR = internal/desktopui/assets/brand
RSVG_CONVERT ?= rsvg-convert
WINDOWS_ICON_ICO = $(BUILD_PATH)/glance-windows-amd64.ico

.PHONY: brand-assets macos-app macos-portable macos-portable-arm64 macos-portable-amd64

brand-assets:
	@set -eu; \
	if command -v "$(RSVG_CONVERT)" >/dev/null 2>&1; then \
		"$(RSVG_CONVERT)" --width 512 --height 512 --output "$(BRAND_ASSETS_DIR)/glance-dock-icon.png" "$(BRAND_ASSETS_DIR)/glance-dock-icon.svg"; \
		"$(RSVG_CONVERT)" --width 22 --height 22 --output "$(BRAND_ASSETS_DIR)/glance-tray-macos-22.png" "$(BRAND_ASSETS_DIR)/glance-tray-macos-22.svg"; \
		"$(RSVG_CONVERT)" --width 44 --height 44 --output "$(BRAND_ASSETS_DIR)/glance-tray-macos-44.png" "$(BRAND_ASSETS_DIR)/glance-tray-macos-44.svg"; \
	else \
		echo "rsvg-convert not found; using the checked-in PNG brand assets" >&2; \
	fi

macos-app:
	@test "$(shell go env GOOS)" = "darwin" || (echo "macOS app packaging requires a macOS host" >&2; exit 1)
	$(MAKE) brand-assets
	mkdir -p "$(MACOS_APP_OUTPUT)/Contents/MacOS" "$(MACOS_APP_OUTPUT)/Contents/Resources"
	GOOS=darwin GOARCH=$(MACOS_ARCH) CGO_ENABLED=1 go build -ldflags "$(MACOS_LINKER_FLAGS)" -trimpath -tags netgo -o "$(MACOS_APP_OUTPUT)/Contents/MacOS/Glance" .
	cp packaging/macos/Info.plist "$(MACOS_APP_OUTPUT)/Contents/Info.plist"
	bash packaging/macos/build-icon.sh "$(MACOS_DOCK_ICON)" "$(MACOS_APP_OUTPUT)/Contents/Resources/Glance.icns" "$(MACOS_ICONSET)"
	$(MAKE) render-runtime RENDER_RUNTIME_TARGET=darwin-$(MACOS_ARCH)
	rm -rf "$(MACOS_APP_OUTPUT)/Contents/Resources/render-runtime"
	cp -R "$(BUILD_PATH)/.render-runtime/darwin-$(MACOS_ARCH)" "$(MACOS_APP_OUTPUT)/Contents/Resources/render-runtime"
	go run ./packaging/render-runtime -check -target darwin-$(MACOS_ARCH) -output "$(MACOS_APP_OUTPUT)/Contents/Resources/render-runtime"
	cp packaging/notices/THIRD_PARTY_NOTICES.md "$(MACOS_APP_OUTPUT)/Contents/Resources/THIRD_PARTY_NOTICES.md"

macos-portable: macos-app
	ditto -c -k --sequesterRsrc --keepParent "$(MACOS_APP_OUTPUT)" "$(BUILD_PATH)/Glance-macos-$(MACOS_ARCH)-portable.zip"

macos-portable-arm64:
	$(MAKE) macos-app MACOS_ARCH=arm64 MACOS_APP_OUTPUT=$(BUILD_PATH)/.macos-stage/arm64/Glance.app
	ditto -c -k --sequesterRsrc --keepParent "$(BUILD_PATH)/.macos-stage/arm64/Glance.app" "$(BUILD_PATH)/Glance-macos-arm64-portable.zip"

macos-portable-amd64:
	$(MAKE) macos-app MACOS_ARCH=amd64 MACOS_APP_OUTPUT=$(BUILD_PATH)/.macos-stage/amd64/Glance.app
	ditto -c -k --sequesterRsrc --keepParent "$(BUILD_PATH)/.macos-stage/amd64/Glance.app" "$(BUILD_PATH)/Glance-macos-amd64-portable.zip"

include packaging/render-runtime/runtime.mk

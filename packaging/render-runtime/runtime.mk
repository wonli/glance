# Verified build-time runtime staging. Installed applications never fetch assets.
RENDER_RUNTIME_TARGET ?= $(shell go env GOOS)-$(shell go env GOARCH)
RENDER_RUNTIME_ARCHIVE ?=
RENDER_RUNTIME_STAGED = $(BUILD_PATH)/.render-runtime/$(RENDER_RUNTIME_TARGET)

.PHONY: render-runtime windows-portable
render-runtime: $(RENDER_RUNTIME_STAGED)/runtime.json
	@if go run ./packaging/render-runtime -check -target "$(RENDER_RUNTIME_TARGET)" -output "$(RENDER_RUNTIME_STAGED)"; then \
		:; \
	else \
		echo "staged browser runtime is incomplete; rebuilding it" >&2; \
		rm -rf "$(RENDER_RUNTIME_STAGED)"; \
		$(MAKE) "$(RENDER_RUNTIME_STAGED)/runtime.json"; \
		go run ./packaging/render-runtime -check -target "$(RENDER_RUNTIME_TARGET)" -output "$(RENDER_RUNTIME_STAGED)"; \
	fi

$(RENDER_RUNTIME_STAGED)/runtime.json: packaging/render-runtime/manifest.json packaging/render-runtime/main.go
	@mkdir -p "$(BUILD_PATH)" "$(dir $(RENDER_RUNTIME_STAGED))"
	@set -eu; \
	stage=$$(mktemp -d "$(BUILD_PATH)/.render-stage.XXXXXX"); \
	trap 'rm -rf "$$stage"' EXIT; \
	go run ./packaging/render-runtime -target "$(RENDER_RUNTIME_TARGET)" -output "$$stage/bundle" -archive "$(RENDER_RUNTIME_ARCHIVE)"; \
	rm -rf "$(RENDER_RUNTIME_STAGED)"; \
	mv "$$stage/bundle" "$(RENDER_RUNTIME_STAGED)"

windows-portable: windows
	$(MAKE) render-runtime RENDER_RUNTIME_TARGET=windows-amd64
	mkdir -p "$(BUILD_PATH)/Glance-windows-amd64"
	cp "$(BUILD_PATH)/$(APP_NAME)-windows-amd64-latest.exe" "$(BUILD_PATH)/Glance-windows-amd64/Glance.exe"
	rm -rf "$(BUILD_PATH)/Glance-windows-amd64/render-runtime"
	cp -R "$(BUILD_PATH)/.render-runtime/windows-amd64" "$(BUILD_PATH)/Glance-windows-amd64/render-runtime"
	go run ./packaging/render-runtime -check -target windows-amd64 -output "$(BUILD_PATH)/Glance-windows-amd64/render-runtime"
	cp packaging/notices/THIRD_PARTY_NOTICES.md "$(BUILD_PATH)/Glance-windows-amd64/THIRD_PARTY_NOTICES.md"
	go run ./packaging/portable -root "$(BUILD_PATH)/Glance-windows-amd64" -output "$(BUILD_PATH)/Glance-windows-amd64-portable.zip"

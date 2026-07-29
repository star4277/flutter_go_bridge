APP_NAME := flutter_go_bridge_codegen
VERSION ?= 0.1.0
CGO_ENABLED ?= 0

BUILD_ROOT ?= build
# Final archives and uncompressed binaries are written directly to build/.
DIST_DIR ?= $(BUILD_ROOT)
GOCACHE ?= $(CURDIR)/$(BUILD_ROOT)/.tmp/$(TARGET_SYSTEM)-$(TARGET_ARCH)/.gocache

# NO_COMPRESS=1 outputs the executable directly. COMPRESS=0 is also accepted.
NO_COMPRESS ?= 0
COMPRESS ?= 1
NO_COMPRESS_ENABLED := $(or $(filter 1 true yes,$(NO_COMPRESS)),$(filter 0 false no,$(COMPRESS)))

# Release matrix. macOS uses Go's darwin target internally. OpenHarmony is
# included only when the installed Go toolchain advertises it in `go tool dist list`.
GO_DIST_LIST := $(shell go tool dist list)
OPENHARMONY_GO_ARCHS := $(foreach arch,arm64 amd64,$(if $(filter openharmony/$(arch),$(GO_DIST_LIST)),$(arch)))

PLATFORMS := windows linux macos $(if $(OPENHARMONY_GO_ARCHS),openharmony,)
ARCHS_windows := amd64 arm64 386
ARCHS_linux := amd64 arm64 386 arm
ARCHS_macos := amd64 arm64
ARCHS_openharmony := $(OPENHARMONY_GO_ARCHS)

GOOS_windows := windows
GOOS_linux := linux
GOOS_macos := darwin
GOOS_openharmony := openharmony

ALL_RELEASE_TARGETS := $(foreach platform,$(PLATFORMS),$(foreach arch,$(ARCHS_$(platform)),$(platform)-$(arch)))

LDFLAGS ?= -s -w -X main.version=$(VERSION)

EXECUTABLE_EXT = $(if $(filter windows,$(TARGET_SYSTEM)),.exe,)
ARCHIVE_EXT = $(if $(filter windows,$(TARGET_SYSTEM)),zip,tgz)
EXECUTABLE = $(APP_NAME)$(EXECUTABLE_EXT)
BUILD_DIR = $(BUILD_ROOT)/.tmp/$(TARGET_SYSTEM)-$(TARGET_ARCH)
EXECUTABLE_PATH = $(BUILD_DIR)/$(EXECUTABLE)
OUTPUT_BASENAME = $(APP_NAME)-$(TARGET_ARCH)-$(TARGET_SYSTEM)-$(VERSION)
DIRECT_OUTPUT = $(DIST_DIR)/$(OUTPUT_BASENAME)$(EXECUTABLE_EXT)
ARCHIVE_OUTPUT = $(DIST_DIR)/$(OUTPUT_BASENAME).$(ARCHIVE_EXT)

.DEFAULT_GOAL := help

.PHONY: all windows linux macos darwin openharmony clean help list package-one $(ALL_RELEASE_TARGETS)

all: $(ALL_RELEASE_TARGETS)

windows: $(foreach arch,$(ARCHS_windows),windows-$(arch))

linux: $(foreach arch,$(ARCHS_linux),linux-$(arch))

macos: $(foreach arch,$(ARCHS_macos),macos-$(arch))

darwin: macos

openharmony: $(foreach arch,$(ARCHS_openharmony),openharmony-$(arch))
ifeq ($(strip $(OPENHARMONY_GO_ARCHS)),)
	@echo "Skipping OpenHarmony: this Go toolchain does not support openharmony."
endif

define RELEASE_RULE
$(1)-$(2):
	$$(MAKE) --no-print-directory package-one TARGET_SYSTEM=$(1) TARGET_OS=$$(GOOS_$(1)) TARGET_ARCH=$(2)
endef

$(foreach platform,$(PLATFORMS),$(foreach arch,$(ARCHS_$(platform)),$(eval $(call RELEASE_RULE,$(platform),$(arch)))))

# Keep explicit OpenHarmony targets friendly on toolchains without support.
openharmony-%:
	@echo "Skipping $@: this Go toolchain does not support openharmony/$*."

# Go users may naturally type darwin-*; keep these as aliases for macos-*.
.PHONY: darwin-amd64 darwin-arm64
darwin-amd64: macos-amd64
darwin-arm64: macos-arm64

package-one:
ifeq ($(OS),Windows_NT)
	powershell.exe -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BUILD_DIR)', '$(DIST_DIR)', '$(GOCACHE)' | Out-Null"
	powershell.exe -NoProfile -Command "$$env:GOOS='$(TARGET_OS)'; $$env:GOARCH='$(TARGET_ARCH)'; $$env:CGO_ENABLED='$(CGO_ENABLED)'; $$env:GOCACHE='$(GOCACHE)'; go build -trimpath -ldflags '$(LDFLAGS)' -o '$(EXECUTABLE_PATH)' ./command"
else
	mkdir -p "$(BUILD_DIR)" "$(DIST_DIR)" "$(GOCACHE)"
	GOOS="$(TARGET_OS)" GOARCH="$(TARGET_ARCH)" CGO_ENABLED="$(CGO_ENABLED)" GOCACHE="$(GOCACHE)" go build -trimpath -ldflags "$(LDFLAGS)" -o "$(EXECUTABLE_PATH)" ./command
endif
ifneq ($(NO_COMPRESS_ENABLED),)
ifeq ($(OS),Windows_NT)
	powershell.exe -NoProfile -Command "Copy-Item -LiteralPath '$(EXECUTABLE_PATH)' -Destination '$(DIRECT_OUTPUT)' -Force"
else
	cp -f "$(EXECUTABLE_PATH)" "$(DIRECT_OUTPUT)"
endif
	@echo Built: $(DIRECT_OUTPUT)
else
ifeq ($(TARGET_SYSTEM),windows)
ifeq ($(OS),Windows_NT)
	powershell.exe -NoProfile -Command "Compress-Archive -LiteralPath '$(EXECUTABLE_PATH)' -DestinationPath '$(ARCHIVE_OUTPUT)' -Force"
else
	rm -f "$(ARCHIVE_OUTPUT)"
	zip -j -q "$(ARCHIVE_OUTPUT)" "$(EXECUTABLE_PATH)"
endif
else
	tar -czf "$(ARCHIVE_OUTPUT)" -C "$(BUILD_DIR)" "$(EXECUTABLE)"
endif
	@echo Built: $(ARCHIVE_OUTPUT)
endif
ifeq ($(OS),Windows_NT)
	powershell.exe -NoProfile -Command "if (Test-Path -LiteralPath '$(BUILD_DIR)') { Remove-Item -LiteralPath '$(BUILD_DIR)' -Recurse -Force }"
	powershell.exe -NoProfile -Command "Remove-Item -LiteralPath '$(BUILD_ROOT)/.tmp' -Force -ErrorAction SilentlyContinue"
else
	rm -rf "$(BUILD_DIR)"
	rmdir "$(BUILD_ROOT)/.tmp" 2>/dev/null || true
endif

list:
	@echo $(ALL_RELEASE_TARGETS)

clean:
ifeq ($(OS),Windows_NT)
	powershell.exe -NoProfile -Command "if (Test-Path -LiteralPath '$(BUILD_ROOT)') { Remove-Item -LiteralPath '$(BUILD_ROOT)' -Recurse -Force }"
else
	rm -rf "$(BUILD_ROOT)"
endif

help:
	@echo "make windows-amd64              Build one platform and architecture"
	@echo "make windows                    Build all Windows architectures"
	@echo "make linux                      Build all Linux architectures"
	@echo "make macos                      Build all macOS architectures"
	@echo "make openharmony                Build all OpenHarmony architectures"
	@echo "make all                        Build every platform and architecture"
	@echo "make windows-amd64 NO_COMPRESS=1  Output the executable without compression"
	@echo "make all VERSION=1.2.3          Override the release version"
	@echo "make list                       List all release targets"
	@echo "make clean                      Remove build outputs"

BINARY := epdcal
PKG := ./cmd/epdcal
GO ?= go

PREFIX ?= /usr/local
ETCDIR ?= /etc/epdcal
VARLIB ?= /var/lib/epdcal
SYSTEMD_DIR ?= /etc/systemd/system

.PHONY: all build build-pi build-pi64 build-pi-cgo test run clean install systemd-install webui-build

all: build

# Build Next.js web UI (if npm is available) and copy static export
# into internal/web/static for Go embed.FS.
webui-build:
	@if command -v npm >/dev/null 2>&1; then \
		echo "==> Building webui (Next.js)"; \
		cd webui && npm run build && \
		echo "==> Syncing webui/out -> internal/web/static (for embed)"; \
		rm -rf ../internal/web/static; \
		mkdir -p ../internal/web/static; \
		cp -R out/. ../internal/web/static/; \
	else \
		echo "==> npm not found; skipping web UI build"; \
	fi

build: webui-build
	$(GO) build -o $(BINARY) $(PKG)

build-pi: webui-build
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -o $(BINARY) $(PKG)

build-pi64: webui-build
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BINARY) $(PKG)

# cgo + C EPD 드라이버(DEV_Config.c 등)를 사용하는 Zero 2 W용 빌드 타깃.
# 사전 준비:
#   - internal/epd/c 디렉터리에 Waveshare C 드라이버 소스/헤더를 복사
#   - internal/epd/c/Makefile 에서 libepddrv.a 를 빌드 가능하게 구성
#   - Zero 2 W(trixie) 상에서: sudo apt install lgpio-dev 등 C 드라이버
#     빌드에 필요한 의존성 설치
#
# 사용 예:
#   make build-pi-cgo
#
build-pi-cgo: webui-build internal/epd/c/libepddrv.a
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 $(GO) build -o $(BINARY) $(PKG)

# C EPD 드라이버 정적 라이브러리 빌드 (internal/epd/c/Makefile 에 위임)
internal/epd/c/libepddrv.a:
	$(MAKE) -C internal/epd/c libepddrv.a

test:
	$(GO) test ./...

run: build
	./$(BINARY) --render-only --dump

clean:
	rm -f $(BINARY) black.bin red.bin preview.png

install: build systemd-install

systemd-install:
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	install -d $(ETCDIR)
	chmod 700 $(ETCDIR)
	install -d $(VARLIB)
	chmod 700 $(VARLIB)
	install -d $(SYSTEMD_DIR)
	install -m 0644 systemd/epdcal.service $(SYSTEMD_DIR)/epdcal.service
	@echo "Run 'sudo systemctl daemon-reload && sudo systemctl enable --now epdcal' to start the service."
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
		cd webui && npm run build; \
		cd ..; \
		echo "==> Syncing webui/out -> internal/web/static (for embed)"; \
		rm -rf internal/web/static; \
		mkdir -p internal/web/static; \
		cp -R webui/out/* internal/web/static/; \
		echo "==> Creating webui.zip from internal/web/static"; \
		rm -f webui.zip; \
		zip -r webui.zip internal/web/static; \
	else \
		echo "==> npm not found; skipping web UI build"; \
	fi

build: webui-build
	$(GO) build -o $(BINARY) $(PKG)

# Pi용 빌드는 webui.zip 을 ./internal/web/static 으로 풀어서 사용한다.
# (webui.zip 은 개발 머신에서 `make webui-build` 로 생성한 뒤 Pi로 복사)
build-pi:
	@if [ -f webui.zip ]; then \
		echo "==> Unpacking webui.zip into internal/web/static"; \
		rm -rf internal/web/static; \
		unzip -oq webui.zip -d .; \
	else \
		echo "==> webui.zip not found; run 'make webui-build' on a dev machine and copy webui.zip here"; \
		exit 1; \
	fi
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -o $(BINARY) $(PKG)

build-pi64:
	@if [ -f webui.zip ]; then \
		echo "==> Unpacking webui.zip into internal/web/static"; \
		rm -rf internal/web/static; \
		unzip -oq webui.zip -d .; \
	else \
		echo "==> webui.zip not found; run 'make webui-build' on a dev machine and copy webui.zip here"; \
		exit 1; \
	fi
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BINARY) $(PKG)

# cgo + C EPD 드라이버(DEV_Config.c 등)를 사용하는 Zero 2 W용 빌드 타깃.
# webui.zip 을 internal/web/static 으로 풀고, C 드라이버(libepddrv.a)를 링크한다.
#
# 사전 준비:
#   - 개발 머신에서: make webui-build  (webui.zip 생성)
#   - Pi에 webui.zip 과 internal/epd/c 소스 복사 후:
#       make -C internal/epd/c libepddrv.a   (또는 아래 타깃이 자동 실행)
#
build-pi-cgo: internal/epd/c/libepddrv.a
	@if [ -f webui.zip ]; then \
		echo "==> Unpacking webui.zip into internal/web/static"; \
		rm -rf internal/web/static; \
		unzip -oq webui.zip -d .; \
	else \
		echo "==> webui.zip not found; run 'make webui-build' on a dev machine and copy webui.zip here"; \
		exit 1; \
	fi
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
BINARY := epdcal
PKG := ./cmd/epdcal
GO ?= go

PREFIX ?= /usr/local
ETCDIR ?= /etc/epdcal
VARLIB ?= /var/lib/epdcal
SYSTEMD_DIR ?= /etc/systemd/system
SERVICE_USER ?= epdcal
SERVICE_GROUP ?= epdcal

RENDER_SYSTEMD_UNIT = sed \
	-e 's|@PREFIX@|$(PREFIX)|g' \
	-e 's|@BINARY@|$(BINARY)|g' \
	-e 's|@ETCDIR@|$(ETCDIR)|g' \
	-e 's|@VARLIB@|$(VARLIB)|g' \
	-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
	-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g'

.PHONY: all build build-pi build-pi64 build-pi-cgo test run clean install systemd-install webui-build

all: build

# Build Next.js web UI and copy static export
# into internal/web/static for Go embed.FS.
webui-build:
	@command -v npm >/dev/null 2>&1 || { echo "npm is required for webui-build"; exit 1; }
	@test -d webui/node_modules || { echo "Run 'cd webui && npm ci' first"; exit 1; }
	@set -e; \
		echo "==> Building webui (Next.js)"; \
		cd webui && npm run build; \
		cd ..; \
		test -f webui/out/calendar/index.html; \
		echo "==> Syncing webui/out -> internal/web/static (for embed)"; \
		rm -rf internal/web/static; \
		mkdir -p internal/web/static; \
		cp -R webui/out/. internal/web/static/; \
		echo "==> Creating webui.zip from internal/web/static"; \
		rm -f webui.zip; \
		zip -qr webui.zip internal/web/static

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
build-pi-cgo: clib
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
clib:
	$(MAKE) -C internal/epd/c libepddrv.a

test:
	$(GO) test ./...

run: build
	./$(BINARY) --render-only --dump

clean:
	rm -f $(BINARY) black.bin red.bin preview.png

install: build systemd-install

systemd-install:
	# Ensure the service group exists before creating the dedicated user.
	@if ! getent group $(SERVICE_GROUP) >/dev/null 2>&1; then \
		if command -v groupadd >/dev/null 2>&1; then \
			groupadd --system $(SERVICE_GROUP); \
		elif command -v addgroup >/dev/null 2>&1; then \
			addgroup --system $(SERVICE_GROUP); \
		else \
			echo "No groupadd/addgroup found; create system group '$(SERVICE_GROUP)' manually."; \
			exit 1; \
		fi; \
	fi
	@if ! id -u $(SERVICE_USER) >/dev/null 2>&1; then \
		if command -v useradd >/dev/null 2>&1; then \
			useradd --system --no-create-home --gid $(SERVICE_GROUP) --shell /usr/sbin/nologin $(SERVICE_USER); \
		elif command -v adduser >/dev/null 2>&1; then \
			adduser --system --no-create-home --disabled-login --ingroup $(SERVICE_GROUP) --shell /usr/sbin/nologin $(SERVICE_USER); \
		else \
			echo "No useradd/adduser found; create system user '$(SERVICE_USER)' manually."; \
			exit 1; \
		fi; \
	fi

	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	install -d $(ETCDIR)
	chown $(SERVICE_USER):$(SERVICE_GROUP) $(ETCDIR)
	chmod 700 $(ETCDIR)
	# Create a sample config on first install. Keep permission 0600 since it can
	# contain secrets (ICS private URLs, basic auth).
	@if [ ! -f $(ETCDIR)/config.yaml ]; then \
		install -m 0600 systemd/config.yaml.sample $(ETCDIR)/config.yaml; \
		chown $(SERVICE_USER):$(SERVICE_GROUP) $(ETCDIR)/config.yaml; \
	fi
	install -d $(VARLIB)
	install -d $(VARLIB)/ics-cache
	chown -R $(SERVICE_USER):$(SERVICE_GROUP) $(VARLIB)
	chmod 700 $(VARLIB)
	chmod 700 $(VARLIB)/ics-cache
	install -d $(SYSTEMD_DIR)
	$(RENDER_SYSTEMD_UNIT) systemd/epdcal.service > $(SYSTEMD_DIR)/epdcal.service
	@if command -v systemctl >/dev/null 2>&1; then \
		systemctl daemon-reload; \
		if systemctl is-active --quiet epdcal; then \
			systemctl restart epdcal; \
			echo "==> Restarted epdcal so the newly installed binary is running."; \
		else \
			systemctl enable epdcal >/dev/null 2>&1 || true; \
			echo "==> Installed epdcal. Start it with: sudo systemctl start epdcal"; \
		fi; \
	else \
		echo "Run 'sudo systemctl daemon-reload && sudo systemctl restart epdcal' to load the new binary."; \
	fi

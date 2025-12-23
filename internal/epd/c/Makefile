# Makefile for building libepddrv.a (Waveshare EPD C driver)
#
# 전제:
#   - 이 디렉터리에 Waveshare C 드라이버 소스가 존재한다고 가정한다.
#   - 최소 필요 파일 예시:
#       DEV_Config.c
#       DEV_Config.h
#       EPD_12in48B.c
#       EPD_12in48B.h
#   - 함수 시그니처는 Go cgo 래퍼에서 사용 중인 것과 일치해야 한다:
#       UBYTE DEV_ModuleInit(void);
#       void  DEV_ModuleExit(void);
#       void  EPD_12in48B_Init(void);
#       void  EPD_12in48B_Clear(void);
#       void  EPD_12in48B_Display(const unsigned char *black,
#                                 const unsigned char *red);
#       void  EPD_12in48B_Sleep(void);
#
# 사용 예:
#   cd internal/epd/c
#   make            # libepddrv.a 생성
#
# 루트 Makefile에서는:
#   make build-pi-cgo
# 가 이 Makefile의 libepddrv.a 타깃을 자동으로 호출한다.

CC      ?= gcc
AR      ?= ar
CFLAGS  ?= -O2 -Wall
CFLAGS  += -I.
CFLAGS  += -DUSE_LGPIO_LIB

# 필요에 따라 소스 파일 목록을 수정하세요.
# 현재 디렉터리 실제 파일 이름:
#   DEV_Config.c
#   EPD_12in48b.c
SRCS = \
	DEV_Config.c \
	EPD_12in48b.c

OBJS = $(SRCS:.c=.o)
LIB  = libepddrv.a

.PHONY: all clean

all: $(LIB)

$(LIB): $(OBJS)
	$(AR) rcs $@ $^

%.o: %.c
	$(CC) $(CFLAGS) -c -o $@ $<

clean:
	rm -f $(OBJS) $(LIB)
# Define variables
GO      := go
GOBUILD := $(GO) build
GOCLEAN := $(GO) clean
BINARY  := x-media-downloader

NATIVE_OS   := $(shell $(GO) env GOOS)
NATIVE_ARCH := $(shell $(GO) env GOARCH)

.PHONY: all build build-linux build-mac build-win clean

all: build

build:
	$(GOBUILD) -o $(BINARY)-$(NATIVE_OS)-$(NATIVE_ARCH) .

build-mac:
	GOOS=darwin GOARCH=$(NATIVE_ARCH) $(GOBUILD) -o $(BINARY)-mac .

build-linux:
	GOOS=linux GOARCH=$(NATIVE_ARCH) $(GOBUILD) -o $(BINARY)-linux .

 build-win:
 	GOOS=windows GOARCH=$(NATIVE_ARCH) $(GOBUILD) -o $(BINARY)-win.exe .

clean:
	$(GOCLEAN)
	rm -f $(BINARY)-darwin-amd64 $(BINARY)-darwin-arm64 \
	      $(BINARY)-linux-amd64  $(BINARY)-linux-arm64  \
	      $(BINARY)-mac $(BINARY)-linux $(BINARY)-win.exe \
	      x-media-downloader

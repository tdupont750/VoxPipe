# CPU-only build. libwhisper is built statically into build/whisper, then cgo is pointed at it.
WHISPER_DIR := $(CURDIR)/third_party/whisper.cpp
BUILD_DIR   := $(CURDIR)/build/whisper
JOBS        ?= $(shell nproc)
GOLANGCI_LINT_VERSION ?= v2.1.6

export C_INCLUDE_PATH := $(WHISPER_DIR)/include:$(WHISPER_DIR)/ggml/include
export LIBRARY_PATH   := $(BUILD_DIR)/src:$(BUILD_DIR)/ggml/src

# Every Go file in the module, excluding the vendored whisper.cpp bindings.
GOFILES := $(shell find . -name '*.go' -not -path './third_party/*')

.PHONY: all libwhisper build test test-race vet lint fmt fmt-check run clean

all: build

$(BUILD_DIR)/src/libwhisper.a:
	git submodule update --init --recursive third_party/whisper.cpp
	cmake -S $(WHISPER_DIR) -B $(BUILD_DIR) -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=OFF
	cmake --build $(BUILD_DIR) --target whisper -j $(JOBS)

libwhisper: $(BUILD_DIR)/src/libwhisper.a

build: libwhisper
	go build -o bin/voxpipe ./cmd/voxpipe

test: libwhisper
	go test ./...

# The audio callback, the command fan-in and the HTTP source all share state, so the race
# detector is where concurrency bugs in this project actually surface.
test-race: libwhisper
	go test -race ./...

vet: libwhisper
	go vet ./...

# Uses a locally installed golangci-lint if present, otherwise runs it with `go run`.
lint: libwhisper
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...; \
	fi

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@unformatted=$$(gofmt -l $(GOFILES)); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

run: build
	./bin/voxpipe

clean:
	rm -rf build bin

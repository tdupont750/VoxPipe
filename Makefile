# CPU-only build. libwhisper is built statically into build/whisper, then cgo is pointed at it.
WHISPER_DIR := $(CURDIR)/third_party/whisper.cpp
BUILD_DIR   := $(CURDIR)/build/whisper
JOBS        ?= $(shell nproc)

export C_INCLUDE_PATH := $(WHISPER_DIR)/include:$(WHISPER_DIR)/ggml/include
export LIBRARY_PATH   := $(BUILD_DIR)/src:$(BUILD_DIR)/ggml/src

.PHONY: all libwhisper build test vet run clean

all: build

$(BUILD_DIR)/src/libwhisper.a:
	git submodule update --init --recursive third_party/whisper.cpp
	cmake -S $(WHISPER_DIR) -B $(BUILD_DIR) -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=OFF
	cmake --build $(BUILD_DIR) --target whisper -j $(JOBS)

libwhisper: $(BUILD_DIR)/src/libwhisper.a

build: libwhisper
	go build -o bin/gowhisper ./cmd/gowhisper

test: libwhisper
	go test ./...

vet: libwhisper
	go vet ./...

run: build
	./bin/gowhisper

clean:
	rm -rf build bin

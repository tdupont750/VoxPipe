module gowhisper

go 1.24.5

require github.com/dlclark/regexp2 v1.12.0

require (
	github.com/gen2brain/malgo v0.11.28
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0
)

replace github.com/ggerganov/whisper.cpp/bindings/go => ./third_party/whisper.cpp/bindings/go

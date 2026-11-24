# Development container

This folder holds the container image the author develops VoxPipe in. **You do not need
it to build or run VoxPipe.** The app is built on the host with `make build`; see the
root [README.md](../README.md) for the real prerequisites.

## What it is

`Containerfile` builds an Ubuntu 24.04 image with the Go toolchain, the cgo build
dependencies for whisper.cpp and malgo (`build-essential`, `cmake`, `libasound2-dev`),
[Claude Code](https://claude.com/product/claude-code), and a couple of TUI conveniences
(lazygit, superfile).

```bash
podman build -f .devcontainer/Containerfile -t voxpipe-dev .
```

Audio capture does not work in the container: there is no microphone or ALSA device
inside it. Builds, unit tests and `make vet` do. Run the app itself on the host.

## clause

The image is launched by `clause`, a personal wrapper around the Claude Code CLI that is
not part of this repo and is not publicly available. It accounts for the parts of the
Containerfile that look unexplained:

- the `claude` user (uid 1000) and its passwordless sudo rule
- `alias clause="claude ${CLAUSE_ARGS}"`, where `CLAUSE_ARGS` is injected at launch
- the `clause-nested-begin` / `clause-nested-end` markers, which fence an optional
  nested-podman layer that `clause podman enable|disable` comments in and out in place

Without the wrapper the image still builds and `bash` still works; you just launch
`claude` directly instead of `clause`.

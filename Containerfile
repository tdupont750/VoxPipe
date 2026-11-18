FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    curl \
    git \
    git-lfs \
    ca-certificates \
    jq \
    python3 \
    unzip \
    zip \
    libicu74 \
    build-essential \
    cmake \
    pkg-config \
    libasound2-dev \
    sudo \
    postgresql \
    postgresql-client \
    sudo \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# .NET SDK 10 — repo pins 10.0.100 in global.json. The Ubuntu 24.04 feed only ships
# .NET 8 LTS, so install the pinned SDK via the official dotnet-install script, system-wide.
RUN curl -fsSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh \
    && bash /tmp/dotnet-install.sh --version 10.0.100 --install-dir /usr/share/dotnet \
    && ln -sf /usr/share/dotnet/dotnet /usr/local/bin/dotnet \
    && rm -f /tmp/dotnet-install.sh
ENV DOTNET_ROOT=/usr/share/dotnet

# Go toolchain for the GoWhisper port. Ubuntu 24.04 ships Go 1.22, so install the
# official tarball. build-essential/cmake/libasound2-dev above cover cgo (whisper.cpp, malgo).
ARG GO_VERSION=1.24.5
RUN arch="$(dpkg --print-architecture)" \
    && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" | tar -xz -C /usr/local

# AWS CLI v2 — deploy workflow uses `aws s3 cp`, the CloudFront prefix-list lookup, and
# provides the credentials `cdk deploy` reaches AWS with. (uname -m → x86_64 | aarch64.)
RUN ARCH=$(uname -m) \
    && curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}.zip" -o /tmp/awscliv2.zip \
    && unzip -q /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install \
    && rm -rf /tmp/aws /tmp/awscliv2.zip

# SSM Session Manager plugin — production RDS is private (PRIVATE_ISOLATED subnet, no
# public access). The only way to reach it is an SSM port-forward through the app EC2 box
# (`aws ssm start-session ... AWS-StartPortForwardingSessionToRemoteHost`), then psql over
# the tunnel. The plugin is required for any interactive `aws ssm start-session`.
# (uname -m → x86_64 maps to ubuntu_64bit, aarch64 → ubuntu_arm64.)
RUN case "$(uname -m)" in \
        x86_64) SSM_ARCH=ubuntu_64bit ;; \
        aarch64) SSM_ARCH=ubuntu_arm64 ;; \
        *) echo "unsupported arch $(uname -m)" >&2; exit 1 ;; \
    esac \
    && curl -fsSL "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/${SSM_ARCH}/session-manager-plugin.deb" -o /tmp/session-manager-plugin.deb \
    && dpkg -i /tmp/session-manager-plugin.deb \
    && rm -f /tmp/session-manager-plugin.deb

RUN arch="$(dpkg --print-architecture)" \
    && case "$arch" in amd64) larch=x86_64 ;; arm64) larch=arm64 ;; *) echo "unsupported arch: $arch" >&2; exit 1 ;; esac \
    && tag="$(curl -fsSLo /dev/null -w '%{url_effective}' https://github.com/jesseduffield/lazygit/releases/latest)" \
    && tag="${tag##*/}" \
    && curl -fsSL "https://github.com/jesseduffield/lazygit/releases/download/${tag}/lazygit_${tag#v}_Linux_${larch}.tar.gz" \
       | tar -xz -C /usr/local/bin lazygit

RUN arch="$(dpkg --print-architecture)" \
    && case "$arch" in amd64|arm64) ;; *) echo "unsupported arch: $arch" >&2; exit 1 ;; esac \
    && tag="$(curl -fsSLo /dev/null -w '%{url_effective}' https://github.com/yorukot/superfile/releases/latest)" \
    && tag="${tag##*/}" \
    && curl -fsSL "https://github.com/yorukot/superfile/releases/download/${tag}/superfile-linux-${tag}-${arch}.tar.gz" \
       | tar -xzO "./dist/superfile-linux-${tag}-${arch}/spf" > /usr/local/bin/spf \
    && chmod 755 /usr/local/bin/spf

RUN userdel -r ubuntu 2>/dev/null || true \
    && useradd -m -u 1000 -s /bin/bash claude \
    && printf 'claude ALL=(ALL) NOPASSWD: ALL\nDefaults logfile=/home/claude/.claude/clause-sudo.log\n' > /etc/sudoers.d/claude \
    && chmod 0440 /etc/sudoers.d/claude

USER claude
ENV HOME=/home/claude
ENV NPM_CONFIG_PREFIX=/home/claude/.npm-global
ENV PATH=/home/claude/.npm-global/bin:$PATH
ENV DOTNET_CLI_TELEMETRY_OPTOUT=1
ENV DOTNET_NOLOGO=1
ENV DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1
ENV PATH=$PATH:/home/claude/.dotnet/tools
ENV PATH=$PATH:/usr/local/go/bin:/home/claude/go/bin
ENV CGO_ENABLED=1
WORKDIR /workspace

RUN npm install -g @anthropic-ai/claude-code aws-cdk

# CLAUSE_ARGS is set by the clause wrapper at launch to the resolved args
# (the line `clause status` shows as launch:); empty or unset means bare claude
RUN echo 'alias clause="claude ${CLAUSE_ARGS}"' >> /home/claude/.bashrc

RUN echo "alias lg='lazygit'" >> /home/claude/.bashrc

RUN echo "alias sf='spf'" >> /home/claude/.bashrc

CMD ["bash"]

# Nested podman support: ships disabled. 'clause podman enable' strips the
# '#~ ' prefixes below and 'clause podman disable' restores them; the lines
# between the markers are toggled in place, so edits there survive toggling.
# clause-nested-begin
#~ USER root
#~ RUN apt-get update && apt-get install -y \
#~     podman \
#~     uidmap \
#~     slirp4netns \
#~     fuse-overlayfs \
#~     podman-compose \
#~     && rm -rf /var/lib/apt/lists/* \
#~     && printf 'claude:1:999\nclaude:1001:64535\n' > /etc/subuid \
#~     && printf 'claude:1:999\nclaude:1001:64535\n' > /etc/subgid \
#~     && printf 'unqualified-search-registries = ["docker.io"]\n' > /etc/containers/registries.conf \
#~     && printf '[engine]\ncgroup_manager = "cgroupfs"\nevents_logger = "file"\n' >> /etc/containers/containers.conf
#~ RUN arch="$(dpkg --print-architecture)" \
#~     && case "$arch" in amd64) larch=x86_64 ;; arm64) larch=arm64 ;; *) echo "unsupported arch: $arch" >&2; exit 1 ;; esac \
#~     && tag="$(curl -fsSLo /dev/null -w '%{url_effective}' https://github.com/jesseduffield/lazydocker/releases/latest)" \
#~     && tag="${tag##*/}" \
#~     && curl -fsSL "https://github.com/jesseduffield/lazydocker/releases/download/${tag}/lazydocker_${tag#v}_Linux_${larch}.tar.gz" \
#~        | tar -xz -C /usr/local/bin lazydocker
#~ USER claude
#~ RUN mkdir -p /home/claude/.local/share/containers
#~ # lazydocker speaks the docker API; compose actions are mapped to podman-compose
#~ RUN mkdir -p /home/claude/.config/lazydocker \
#~     && printf 'commandTemplates:\n  dockerCompose: podman-compose\n' > /home/claude/.config/lazydocker/config.yml
#~ # lazydocker wrapper: start podman's API socket on demand, point DOCKER_HOST at it
#~ # (alias ld shadows the binutils linker in interactive shells only)
#~ RUN printf '%s\n' \
#~     '' \
#~     'lazydocker() {' \
#~     '  local sock' \
#~     '  sock="$(podman info --format "{{.Host.RemoteSocket.Path}}" 2>/dev/null)"' \
#~     '  sock="${sock#unix://}"' \
#~     '  if [ -z "$sock" ]; then' \
#~     '    echo "lazydocker: podman not available; nested podman must be enabled" >&2' \
#~     '    return 1' \
#~     '  fi' \
#~     '  if [ ! -S "$sock" ]; then' \
#~     '    (podman system service --time=0 >/dev/null 2>&1 &)' \
#~     '    local i=0' \
#~     '    while [ "$i" -lt 50 ] && [ ! -S "$sock" ]; do sleep 0.1; i=$((i+1)); done' \
#~     '  fi' \
#~     '  DOCKER_HOST="unix://$sock" command lazydocker "$@"' \
#~     '}' \
#~     "alias ld='lazydocker'" \
#~     >> /home/claude/.bashrc
# clause-nested-end

FROM mcr.microsoft.com/devcontainers/go:1.25-bookworm

ENV GOPROXY=https://goproxy.cn,direct
ENV GOTOOLCHAIN=auto


USER root

RUN if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
        sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources; \
    fi \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
    exiftool \
    libvips42 \
    curl \
    git \
    gh \
 && rm -rf /var/lib/apt/lists/*

# Node 22
RUN curl -fsSL \
https://npmmirror.com/mirrors/node/v22.23.1/node-v22.23.1-linux-x64.tar.xz \
-o /tmp/node.tar.xz \
&& tar -xf /tmp/node.tar.xz -C /usr/local --strip-components=1 \
&& rm /tmp/node.tar.xz


RUN go install github.com/air-verse/air@v1.61.7 \
 && go install golang.org/x/tools/gopls@latest


RUN chmod -R 777 /go
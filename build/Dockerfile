FROM debian:bookworm-slim

ARG GO_VERSION=1.25.3

RUN apt-get update && apt-get install -y --no-install-recommends \
        gcc \
        libc6-dev \
        binutils \
        curl \
        ca-certificates \
        git \
    && rm -rf /var/lib/apt/lists/*

RUN ARCH="$(dpkg --print-architecture)" && \
    case "$ARCH" in \
      amd64) GOTAR="go${GO_VERSION}.linux-amd64.tar.gz" ;; \
      arm64) GOTAR="go${GO_VERSION}.linux-arm64.tar.gz" ;; \
      *) echo "unsupported arch $ARCH" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://go.dev/dl/$GOTAR" -o /tmp/go.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"

# ARM64 LSE atomics fix — see krf_cgo.go header comment for full context.
COPY build/fix-lse.sh /tmp/fix-lse.sh
RUN chmod +x /tmp/fix-lse.sh && /tmp/fix-lse.sh && rm /tmp/fix-lse.sh

WORKDIR /work

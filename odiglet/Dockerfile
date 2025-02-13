FROM --platform=$TARGETPLATFORM apache/skywalking-java-agent:9.3.0-alpine as sw_agent
FROM --platform=$TARGETPLATFORM registry.cn-hangzhou.aliyuncs.com/kindlingx/autoinstrumentation-python:0.48b0 as python_agent

FROM --platform=$TARGETPLATFORM centos:centos7.9.2009 as preload
ARG TARGETPLATFORM

# set up centos-vault mirror
RUN if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
        curl -L -o /etc/yum.repos.d/CentOS-Base.repo http://mirrors.aliyun.com/repo/Centos-7.repo; \
    elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        curl -L -o /etc/yum.repos.d/CentOS-Base.repo http://mirrors.aliyun.com/repo/Centos-altarch-7.repo; \
    else \
        echo "ERROR: Unsupported platform: $TARGETPLATFORM"; \
        exit 1; \
    fi

RUN yum install -y gcc

# setup rust proxy
ENV RUSTUP_DIST_SERVER="https://rsproxy.cn"
ENV RUSTUP_UPDATE_ROOT="https://rsproxy.cn/rustup"
ENV PATH $PATH:/root/.cargo/bin

# Install Rustup
RUN curl --proto '=https' --tlsv1.2 -sSf https://rsproxy.cn/rustup-init.sh | sh -s -- -y
RUN /root/.cargo/bin/rustup default stable

COPY preload /src/preload
WORKDIR /src/preload

RUN mkdir -p /etc/apo/instrument && gcc -std=c99 -o /etc/apo/instrument/libapolanucher.so -shared launcher/lanucher.c -Wall -Wfatal-errors -fPIC -ldl
COPY preload/.apo-config/cargo-proxy.toml /usr/local/cargo/config.toml
RUN cd /src/preload/instrument && cargo build --release  && cp target/release/libapoinstrument.so /etc/apo/instrument/.
COPY preload/.apo-config/* /etc/apo/instrument/.
RUN chmod -R 755 /etc/apo/instrument/.

FROM --platform=$TARGETPLATFORM keyval/odiglet-base:v1.5 as builder
WORKDIR /go/src/github.com/odigos-io/odigos
# Copy local modules required by the build
COPY api/ api/
COPY common/ common/
COPY k8sutils/ k8sutils/
COPY procdiscovery/ procdiscovery/
COPY opampserver/ opampserver/
COPY cli/tools/ini-merger/ /ini-merger/
WORKDIR /go/src/github.com/odigos-io/odigos/odiglet
COPY odiglet/ .

ARG TARGETARCH
ENV GOPROXY https://goproxy.cn

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    GOOS=linux make build-odiglet

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    GOOS=linux go build -o /opt/ini-merger /ini-merger/ini-merger.go

WORKDIR /instrumentations

# Java
ARG JAVA_OTEL_VERSION=v2.8.0
ADD ./opentelemetry-javaagent.jar /instrumentations/java/javaagent.jar
RUN chmod 644 /instrumentations/java/javaagent.jar

# Skywalking JAVA
ARG SW_AGENT_VERSION=9.3.0
COPY --from=sw_agent /skywalking/agent /instrumentations/skywalking/java

# Python
ARG PYTHON_OTEL_VERSION=0.48b0
COPY --from=python_agent /autoinstrumentation /instrumentations/python

# NodeJS
COPY  nodejs-instrumentation/workspace/node_modules/ /instrumentations/nodejs/node_modules
COPY  nodejs-instrumentation/src /instrumentations/nodejs

# .NET
COPY  ./tracer-home /instrumentations/dotnet

RUN chmod -R +x /instrumentations

FROM --platform=$TARGETPLATFORM registry.fedoraproject.org/fedora-minimal:38
COPY --from=builder /go/src/github.com/odigos-io/odigos/odiglet/odiglet /root/odiglet
WORKDIR /instrumentations/
COPY --from=builder /instrumentations/ .
COPY --from=builder /opt/ini-merger /usr/bin/ini-merger
COPY --from=preload /etc/apo /etc/apo
CMD ["/root/odiglet"]

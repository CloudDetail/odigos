FROM apache/skywalking-java-agent:9.3.0-alpine as sw_agent

FROM registry.cn-hangzhou.aliyuncs.com/kindlingx/centos:7 as preload
# setup proxy for cn
ENV RUSTUP_DIST_SERVER=https://mirrors.ustc.edu.cn/rust-static
ENV RUSTUP_UPDATE_ROOT=https://mirrors.ustc.edu.cn/rust-static/rustup

# Install Rustup
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y

COPY preload/.apo-config/cargo-proxy.toml /usr/local/cargo/config.toml

COPY preload /src/preload
WORKDIR /src/preload
RUN mkdir -p /etc/apo/instrument && gcc -std=c99 -o /etc/apo/instrument/libapolanucher.so -shared launcher/lanucher.c -Wall -Wfatal-errors -fPIC -ldl
RUN cd /src/preload/instrument && cargo build --release  && cp target/release/libapoinstrument.so /etc/apo/instrument/.
COPY preload/.apo-config/* /etc/apo/instrument/.
RUN chmod -R 755 /etc/apo/instrument/.

FROM keyval/odiglet-base:v1.4 as builder
WORKDIR /go/src/github.com/odigos-io/odigos
# Copyy local modules required by the build
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
ARG JAVA_OTEL_VERSION=v2.3.0
ADD ./opentelemetry-javaagent.jar /instrumentations/java/javaagent.jar
RUN chmod 644 /instrumentations/java/javaagent.jar

# Skywalking JAVA
COPY --from=sw_agent /skywalking/agent /instrumentations/skywalking/java

# Python
COPY ./python-instrumentation-originx/workspace/ /instrumentations/python

# NodeJS
COPY  nodejs-instrumentation/workspace/node_modules/ /instrumentations/nodejs/node_modules
COPY  nodejs-instrumentation/src /instrumentations/nodejs

# .NET
COPY  ./tracer-home /instrumentations/dotnet

FROM registry.fedoraproject.org/fedora-minimal:38
COPY --from=builder /go/src/github.com/odigos-io/odigos/odiglet/odiglet /root/odiglet
WORKDIR /instrumentations/
COPY --from=builder /instrumentations/ .
COPY --from=builder /opt/ini-merger /usr/bin/ini-merger
COPY --from=preload /etc/apo /etc/apo
CMD ["/root/odiglet"]

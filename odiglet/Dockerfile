FROM apache/skywalking-java-agent:9.3.0-alpine as sw_agent


FROM keyval/odiglet-base:v1.4 as builder
WORKDIR /go/src/github.com/odigos-io/odigos
# Copyy local modules required by the build
COPY api/ api/
COPY common/ common/
COPY k8sutils/ k8sutils/
COPY procdiscovery/ procdiscovery/
COPY opampserver/ opampserver/
WORKDIR /go/src/github.com/odigos-io/odigos/odiglet
COPY odiglet/ .

ARG TARGETARCH
ENV GOPROXY https://goproxy.cn
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    GOOS=linux GOARCH=$TARGETARCH make build-odiglet

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
CMD ["/root/odiglet"]

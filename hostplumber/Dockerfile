# Build the manager binary
FROM golang:1.22 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
FROM alpine:3.16
WORKDIR /

RUN apk add --no-cache bash
RUN apk add --no-cache ethtool
RUN apk add --no-cache pciutils

RUN apk add --no-cache make gcc linux-headers bsd-compat-headers binutils coreutils diffutils && \
    apk add --no-cache gettext bash grep sed texinfo perl util-linux vim build-base libexecinfo libexecinfo-dev && \
    apk add --no-cache abuild bc cmake ncurses-dev ca-certificates wget && \
    apk add numactl-dev alpine-sdk bsd-compat-headers libexecinfo-dev py3-pip py3-elftools && \
    pip3 install meson ninja && \
    wget http://dpdk.org/browse/dpdk-stable/snapshot/dpdk-stable-21.11.3.zip && \
    unzip dpdk-stable-21.11.3.zip 

WORKDIR dpdk-stable-21.11.3
RUN meson build && \
    ninja -C build && \
    ninja -C build install

WORKDIR /
RUN apk add --no-cache autoconf automake libtool && \
    wget https://www.openvswitch.org/releases/openvswitch-2.17.5.tar.gz && \
    gunzip openvswitch-2.17.5.tar.gz && \
    tar -xf openvswitch-2.17.5.tar 

WORKDIR openvswitch-2.17.5 
RUN ./configure  --localstatedir=/var --sysconfdir=/etc --with-dpdk=static  && \
    make && \
    make install

WORKDIR /
COPY --from=builder /workspace/manager .
#USER 65532:65532

ENTRYPOINT ["/manager"]

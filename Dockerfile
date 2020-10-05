# Build the manager binary
FROM golang:1.13 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/luigi-plugins/main.go cmd/luigi-plugins/main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/apply/ pkg/apply/
COPY plugin_templates /etc/plugin_templates

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager cmd/luigi-plugins/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
#WORKDIR /
#COPY --from=builder /workspace/manager .
#USER nonroot:nonroot

FROM alpine:3.7
RUN apk add bash
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /etc/plugin_templates /etc/plugin_templates

ENTRYPOINT ["/manager"]

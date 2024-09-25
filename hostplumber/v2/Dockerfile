# Build the manager binary
FROM golang:1.21 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/hostplumber/main.go cmd/hostplumber/main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager cmd/hostplumber/main.go

FROM alpine:3.19.4
RUN apk add --no-cache bash
RUN apk add --no-cache openvswitch

WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]

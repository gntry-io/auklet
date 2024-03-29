PKG = github.com/gntry-io/auklet
BINNAME = auklet
VERSION = v0.0.1
GOOS = -e GOOS=linux
GOARCH = -e GOARCH=amd64
CGO = -e CGO_ENABLED=0
BUILDIMAGE = golang:1.11.2-alpine
DOCKERRUN = docker run --rm -t -v ${SRC}:/go/src/${PKG} -w /go/src/${PKG} ${GOOS} ${GOARCH} ${CGO} ${BUILDIMAGE}
GOBUILD = GO111MODULE=on go build -a -tags netgo -ldflags '-s -w -extldflags "-static" -X github.com/gntry-io/auklet/cmd/auklet/cmd.gitTag=${GITTAG} -X github.com/gntry-io/auklet/cmd/auklet/cmd.buildUser=${USER} -X github.com/gntry-io/auklet/cmd/auklet/cmd.version=${VERSION} -X github.com/gntry-io/auklet/cmd/auklet/cmd.buildDate=${BUILDDATE}'
UPX = upx --brute bin/${BINNAME}
BUILDDATE = $(shell date '+%Y%m%d-%H%M')
GITTAG = $(shell git rev-parse --short HEAD)
ifeq ($(GITTAG),)
GITTAG := devel
endif
SRC = $(shell pwd)
GOFILES = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

all: clean build

build:
	${DOCKERRUN} ash -c "apk add --no-cache git upx && GO111MODULE=on go mod download && ${GOBUILD} -o bin/${BINNAME} github.com/gntry-io/auklet/cmd/auklet && ${UPX}"

localbuild:
	${GOBUILD} -v -race -o bin/${BINNAME} github.com/gntry-io/auklet/cmd/auklet

upx:
	${UPX}

fmt:
	@gofmt -l -w ${GOFILES}

check:
	@test -z $(shell gofmt -l cmd/auklet/main.go | tee /dev/stderr) || echo "[WARN] Fix formatting issues with 'make fmt'"
	@for d in $$(go list ./... | grep -v /vendor/); do golint $${d}; done
	@go tool vet ${GOFILES}

clean:
	rm -f bin/${BINNAME}
	rm -f bin/${BINNAME}.*

.PHONY: all build localbuild upx clean

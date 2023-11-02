#!/usr/bin/env bash
NAME=cdns
VERSION=$(git describe --tags --long)

if [ -d build ]
then
  rm -rf build
fi
mkdir build

build() {
  filename=$1
  go build -o ./${filename} -v -trimpath -ldflags "-X 'github.com/rnetx/cdns/constant.Version=${VERSION}' -s -w -buildid=" .
  tar -czf ./build/${filename}.tar.gz ${filename} LICENSE README.md
  rm -rf ./${filename}
  sha256sum ./build/${filename}.tar.gz > ./build/${filename}.tar.gz.sha256
  echo "Build $filename OK!!"
}

build_windows() {
  filename=$1
  go build -o ./${filename}.exe -v -trimpath -ldflags "-X 'github.com/rnetx/cdns/constant.Version=${VERSION}' -s -w -buildid=" .
  zip ./build/${filename}.zip ${filename}.exe LICENSE README.md
  rm -rf ./${filename}.exe
  sha256sum ./build/${filename}.zip > ./build/${filename}.zip.sha256
  echo "Build $filename OK!!"
}

# build command
# linux
GOARCH=amd64 GOOS=linux build "${NAME}-linux-amd64"
GOARCH=amd64 GOOS=linux GOAMD64=v3 build "${NAME}-linux-amd64-v3"
GOARCH=arm64 GOOS=linux build "${NAME}-linux-arm64"
GOARCH=arm GOOS=linux GOARM=7 build "${NAME}-linux-armv7"
GOARCH=arm GOOS=linux GOARM=6 build "${NAME}-linux-armv6"
GOARCH=arm GOOS=linux GOARM=5 build "${NAME}-linux-mips"
GOARCH=mips GOMIPS=softfloat GOOS=linux build "${NAME}-linux-mips-softfloat"
GOARCH=mips GOMIPS=hardfloat GOOS=linux build "${NAME}-linux-mips-hardfloat"
GOARCH=mipsle GOMIPS=softfloat GOOS=linux build "${NAME}-linux-mipsle-softfloat"
GOARCH=mipsle GOMIPS=hardfloat GOOS=linux build "${NAME}-linux-mipsle-hardfloat"
GOARCH=mips64 GOOS=linux build "${NAME}-linux-mips64"
GOARCH=mips64le GOOS=linux build "${NAME}-linux-mips64le"
GOARCH=riscv64 GOOS=linux build "${NAME}-linux-riscv64"
# windows
GOARCH=386 GOOS=windows build_windows "${NAME}-windows-386"
GOARCH=amd64 GOOS=windows build_windows "${NAME}-windows-amd64"
GOARCH=amd64 GOOS=windows GOAMD64=v3 build_windows "${NAME}-windows-amd64-v3"
GOARCH=arm64 GOOS=windows build_windows "${NAME}-windows-arm64"
GOARCH=arm GOOS=windows GOARM=7 build_windows "${NAME}-windows-armv7"
# darwin
GOARCH=arm64 GOOS=darwin build "${NAME}-darwin-arm64"
GOARCH=amd64 GOOS=darwin build "${NAME}-darwin-amd64"

echo "Build ALL OK!!"
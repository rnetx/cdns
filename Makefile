NAME = cdns

TAGS = $(shell git describe --tags --long)

build:
	@go build -ldflags "-s -w -X 'github.com/rnetx/cdns/cmd/cdns.Version=$(TAGS)' -buildid=" -o $(NAME) -v .

fmt:
	@gofumpt -l -w .
	@gofmt -s -w .
	@gci write --custom-order -s standard -s "prefix(github.com/rnetx/)" -s "default" .

fmt_install:
	go install -v mvdan.cc/gofumpt@latest
	go install -v github.com/daixiang0/gci@latest

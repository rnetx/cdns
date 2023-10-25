NAME = cdns

build:
	@go build -ldflags "-s -w -buildid=" -o $(NAME) -v .

fmt:
	@gofumpt -l -w .
	@gofmt -s -w .
	@gci write --custom-order -s standard -s "prefix(github.com/rnetx/)" -s "default" .

fmt_install:
	go install -v mvdan.cc/gofumpt@latest
	go install -v github.com/daixiang0/gci@latest

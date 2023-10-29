package singstub

import (
	"fmt"

	"github.com/rnetx/cdns/utils/domain"
)

type Reader struct{}

func (r *Reader) Close() error {
	return nil
}

func (r *Reader) Read(code string) (*domain.DomainSet, error) {
	return nil, fmt.Errorf("sing geosite type is not supported")
}

func OpenReader(path string) (*Reader, []string, error) {
	return nil, nil, fmt.Errorf("sing geosite type is not supported")
}

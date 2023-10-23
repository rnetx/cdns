package listener

import (
	"context"
	"fmt"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
)

type Options struct {
	Tag         string
	Type        string
	DealTimeout time.Duration
	Workflow    string

	UDPOptions  *UDPListenerOptions
	TCPOptions  *TCPListenerOptions
	TLSOptions  *TLSListenerOptions
	HTTPOptions *HTTPListenerOptions
	QUICOptions *QUICListenerOptions
}

type _Options struct {
	Tag         string         `yaml:"tag"`
	Type        string         `yaml:"type"`
	DealTimeout utils.Duration `yaml:"deal-timeout"`
	Workflow    string         `yaml:"workflow"`
}

func (o *Options) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var _o _Options
	err := unmarshal(&_o)
	if err != nil {
		return err
	}
	var data any
	switch _o.Type {
	case UDPListenerType:
		o.UDPOptions = &UDPListenerOptions{}
		data = o.UDPOptions
	case TCPListenerType:
		o.TCPOptions = &TCPListenerOptions{}
		data = o.TCPOptions
	case TLSListenerType:
		o.TLSOptions = &TLSListenerOptions{}
		data = o.TLSOptions
	case HTTPListenerType:
		o.HTTPOptions = &HTTPListenerOptions{}
		data = o.HTTPOptions
	case QUICListenerType:
		o.QUICOptions = &QUICListenerOptions{}
		data = o.QUICOptions
	default:
		return fmt.Errorf("unknown listener type: %s", _o.Type)
	}
	err = unmarshal(data)
	if err != nil {
		return err
	}
	o.Type = _o.Type
	o.Tag = _o.Tag
	o.DealTimeout = time.Duration(_o.DealTimeout)
	o.Workflow = _o.Workflow
	return nil
}

func NewListener(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options Options) (adapter.Listener, error) {
	var (
		l   adapter.Listener
		err error
	)
	switch options.Type {
	case UDPListenerType:
		l, err = NewUDPListener(ctx, core, logger, tag, *options.UDPOptions, options.Workflow)
	case TCPListenerType:
		l, err = NewTCPListener(ctx, core, logger, tag, *options.TCPOptions, options.Workflow)
	case TLSListenerType:
		l, err = NewTLSListener(ctx, core, logger, tag, *options.TLSOptions, options.Workflow)
	case HTTPListenerType:
		l, err = NewHTTPListener(ctx, core, logger, tag, *options.HTTPOptions, options.Workflow)
	case QUICListenerType:
		l, err = NewQUICListener(ctx, core, logger, tag, *options.QUICOptions, options.Workflow)
	default:
		return nil, fmt.Errorf("unknown listener type: %s", options.Type)
	}
	if err != nil {
		return nil, err
	}
	dealTimeout := options.DealTimeout
	if dealTimeout <= 0 {
		dealTimeout = DefaultDealTimeout
	}
	l = &GenericListener{
		dealTimeout: dealTimeout,
		Listener:    l,
	}
	return l, nil
}

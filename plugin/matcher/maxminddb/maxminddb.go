package maxminddb

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"
)

const Type = "maxminddb"

func init() {
	plugin.RegisterPluginMatcher(Type, NewMaxmindDB)
}

type Args struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type MaxmindDB struct {
	ctx            context.Context
	tag            string
	logger         log.Logger
	runningArgsMap map[uint64]map[string]struct{}

	path     string
	dataType string

	reader     *Reader
	reloadLock sync.Mutex
}

func NewMaxmindDB(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error) {
	m := &MaxmindDB{
		ctx:    ctx,
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	if a.Path == "" {
		return nil, fmt.Errorf("mssing path")
	}
	m.path = a.Path
	m.dataType = a.Type
	return m, nil
}

func (m *MaxmindDB) Tag() string {
	return m.tag
}

func (m *MaxmindDB) Type() string {
	return Type
}

func (m *MaxmindDB) Start() error {
	return m.loadRule()
}

func (m *MaxmindDB) Close() error {
	reader := m.reader
	if reader != nil {
		return reader.Close()
	}
	return nil
}

func (m *MaxmindDB) loadRule() error {
	reader, err := OpenMaxmindDBReader(m.path, m.dataType)
	if err != nil {
		return err
	}
	m.logger.Debugf("load maxminddb success: %d", len(reader.reader.Metadata.Languages))
	m.reader = reader
	return nil
}

func (m *MaxmindDB) LoadRunningArgs(_ context.Context, argsID uint64, args any) error {
	var codes utils.Listable[string]
	err := utils.JsonDecode(args, &codes)
	if err != nil {
		return err
	}
	codeMap := make(map[string]struct{})
	for _, code := range codes {
		c := strings.Split(code, ",")
		for _, cc := range c {
			cc = strings.TrimSpace(cc)
			codeMap[cc] = struct{}{}
		}
	}
	if m.runningArgsMap == nil {
		m.runningArgsMap = make(map[uint64]map[string]struct{})
	}
	m.runningArgsMap[argsID] = codeMap
	return nil
}

func (m *MaxmindDB) Match(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint64) (bool, error) {
	respMsg := dnsCtx.RespMsg()
	if respMsg == nil {
		m.logger.DebugfContext(ctx, "resp msg is nil")
		return false, nil
	}
	ips := make([]netip.Addr, 0, len(respMsg.Answer))
	for _, rr := range respMsg.Answer {
		switch ans := rr.(type) {
		case *dns.A:
			ip, ok := netip.AddrFromSlice(ans.A)
			if ok {
				ips = append(ips, ip)
			}
		case *dns.AAAA:
			ip, ok := netip.AddrFromSlice(ans.AAAA)
			if ok {
				ips = append(ips, ip)
			}
		}
	}
	if len(ips) == 0 {
		m.logger.DebugfContext(ctx, "no ips found")
		return false, nil
	}
	codeMap := m.runningArgsMap[argsID]
	for _, ip := range ips {
		codes := m.reader.Lookup(ip)
		if len(codes) > 0 {
			for _, code := range codes {
				_, ok := codeMap[code]
				if ok {
					m.logger.DebugfContext(ctx, "match code: %s", code)
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (m *MaxmindDB) reloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.reloadLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer m.reloadLock.Unlock()
		m.logger.Infof("reload maxminddb rule...")
		err := m.loadRule()
		if err != nil {
			m.logger.Errorf("reload maxminddb rule failed: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			m.logger.Infof("reload maxminddb rule success")
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (m *MaxmindDB) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/reload",
		Methods:     []string{http.MethodGet},
		Description: "reload maxminddb rule.",
		Handler:     m.reloadHandler(),
	})
	return builder.Build()
}

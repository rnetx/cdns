package maxminddb

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
)

const Type = "maxminddb"

func init() {
	plugin.RegisterPluginMatcher(Type, NewMaxmindDB)
}

type Args struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type runningArgs struct {
	Code        utils.Listable[string] `json:"code"`
	UseClientIP bool                   `json:"use-client-ip"`
}

type runningArgItem struct {
	code        map[string]struct{}
	useClientIP bool
}

var (
	_ adapter.PluginMatcher = (*MaxmindDB)(nil)
	_ adapter.Starter       = (*MaxmindDB)(nil)
	_ adapter.Closer        = (*MaxmindDB)(nil)
	_ adapter.APIHandler    = (*MaxmindDB)(nil)
)

type MaxmindDB struct {
	ctx            context.Context
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16]runningArgItem

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
		return nil, fmt.Errorf("missing path")
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
	oldReader := m.reader
	m.reader = reader
	if oldReader != nil {
		oldReader.Close()
	}
	return nil
}

func (m *MaxmindDB) LoadRunningArgs(_ context.Context, args any) (uint16, error) {
	var codes utils.Listable[string]
	var useClientIP bool
	err := utils.JsonDecode(args, &codes)
	if err != nil {
		var a runningArgs
		err2 := utils.JsonDecode(args, &a)
		if err2 != nil {
			return 0, fmt.Errorf("%w | %w", err, err2)
		}
		codes = a.Code
		useClientIP = a.UseClientIP
	}
	if len(codes) == 0 {
		return 0, fmt.Errorf("missing code")
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
		m.runningArgsMap = make(map[uint16]runningArgItem)
	}
	var id uint16
	for {
		id = utils.RandomIDUint16()
		if _, ok := m.runningArgsMap[id]; !ok {
			break
		}
	}
	m.runningArgsMap[id] = runningArgItem{
		code:        codeMap,
		useClientIP: useClientIP,
	}
	return id, nil
}

func (m *MaxmindDB) Match(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (bool, error) {
	reader := m.reader
	if reader != nil {
		reader = reader.Clone()
	}
	defer reader.Close()
	codeItem := m.runningArgsMap[argsID]
	if codeItem.useClientIP {
		clientIP := dnsCtx.ClientIP()
		codes := reader.Lookup(clientIP)
		if len(codes) > 0 {
			for _, code := range codes {
				_, ok := codeItem.code[code]
				if ok {
					m.logger.DebugfContext(ctx, "match code: %s", code)
					return true, nil
				}
			}
		}
		return false, nil
	}
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
	for _, ip := range ips {
		codes := reader.Lookup(ip)
		if len(codes) > 0 {
			for _, code := range codes {
				_, ok := codeItem.code[code]
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

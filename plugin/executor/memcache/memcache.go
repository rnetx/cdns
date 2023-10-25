package memcache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
)

const Type = "memcache"

func init() {
	plugin.RegisterPluginExecutor(Type, NewMemCache)
}

type Args struct {
	DumpPath     string   `json:"dump-path"`
	DumpInterval Duration `json:"dump-interval"`
	UseExpired   bool     `json:"use-expired"`
	FlushExpired bool     `json:"flush-expired"`
}

type runningArgs struct {
	Mode   string `json:"mode"`
	Return any    `json:"return"`
}

type cacheItem struct {
	Upstream string
	Resp     *dns.Msg
}

type _cacheItem struct {
	Upstream string `json:"upstream"`
	Resp     string `json:"resp"`
}

func (c *cacheItem) UnmarshalJSON(data []byte) error {
	var _c _cacheItem
	err := json.Unmarshal(data, &_c)
	if err != nil {
		return err
	}
	respRaw, err := base64.StdEncoding.DecodeString(_c.Resp)
	if err != nil {
		return err
	}
	c.Resp = &dns.Msg{}
	err = c.Resp.Unpack(respRaw)
	if err != nil {
		return err
	}
	c.Upstream = _c.Upstream
	return nil
}

func (c cacheItem) MarshalJSON() ([]byte, error) {
	var _c _cacheItem
	_c.Upstream = c.Upstream
	respRaw, err := c.Resp.Pack()
	if err != nil {
		return nil, err
	}
	_c.Resp = base64.StdEncoding.EncodeToString(respRaw)
	return json.Marshal(_c)
}

var (
	_ adapter.PluginExecutor = (*MemCache)(nil)
	_ adapter.Starter        = (*MemCache)(nil)
	_ adapter.Closer         = (*MemCache)(nil)
	_ adapter.APIHandler     = (*MemCache)(nil)
)

type MemCache struct {
	ctx            context.Context
	core           adapter.Core
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16]runningArgs

	dumpPath     string
	dumpInterval time.Duration
	useExpired   bool
	flushExpired bool

	dumpLock       sync.Mutex
	cacheMap       *CacheMap[cacheItem]
	loopDumpCtx    context.Context
	loopDumpCancel context.CancelFunc
	closeDone      chan struct{}
}

func NewMemCache(ctx context.Context, core adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
	m := &MemCache{
		ctx:    ctx,
		core:   core,
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	m.dumpPath = a.DumpPath
	m.dumpInterval = time.Duration(a.DumpInterval)
	m.useExpired = a.UseExpired
	m.flushExpired = a.FlushExpired
	return m, nil
}

func (m *MemCache) Tag() string {
	return m.tag
}

func (m *MemCache) Type() string {
	return Type
}

func (m *MemCache) Start() error {
	if m.dumpPath != "" {
		raw, err := os.ReadFile(m.dumpPath)
		if err != nil {
			return fmt.Errorf("load dump file failed: %s, error: %s", m.dumpPath, err)
		}
		cacheMap, err := Decode[cacheItem](m.ctx, raw)
		if err != nil {
			return fmt.Errorf("load dump file failed: %s, error: %s", m.dumpPath, err)
		}
		m.cacheMap = cacheMap
	} else {
		m.cacheMap = NewCacheMap[cacheItem](m.ctx)
	}
	m.cacheMap.Start()
	if m.dumpPath != "" && m.dumpInterval > 0 {
		m.loopDumpCtx, m.loopDumpCancel = context.WithCancel(m.ctx)
		m.closeDone = make(chan struct{}, 1)
		go m.loopDump()
	}
	return nil
}

func (m *MemCache) Close() error {
	if m.dumpPath != "" && m.dumpInterval > 0 {
		m.loopDumpCancel()
		<-m.closeDone
		close(m.closeDone)
	}
	cacheMap := m.cacheMap
	if cacheMap != nil {
		if m.dumpPath != "" {
			err := dump(cacheMap, m.dumpPath)
			if err != nil {
				m.logger.Errorf("dump cache failed: %s", err)
			}
		}
		cacheMap.Close()
	}
	return nil
}

func (m *MemCache) loopDump() {
	defer func() {
		select {
		case m.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(m.dumpInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.loopDumpCtx.Done():
			return
		case <-ticker.C:
			m.dumpLock.Lock()
			err := dump(m.cacheMap, m.dumpPath)
			if err != nil {
				m.logger.Errorf("dump cache failed: %s", err)
			}
			m.dumpLock.Unlock()
		}
	}
}

func (m *MemCache) LoadRunningArgs(_ context.Context, args any) (uint16, error) {
	var a runningArgs
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return 0, fmt.Errorf("parse args failed: %w", err)
	}
	switch a.Mode {
	case "restore":
	case "store":
	default:
		return 0, fmt.Errorf("unknown mode: %s", a.Mode)
	}
	switch r := a.Return.(type) {
	case string:
		switch r {
		case "All", "all":
			a.Return = "all"
		case "Once", "once":
			a.Return = "once"
		default:
			return 0, fmt.Errorf("unknown return: %s", r)
		}
	case bool:
		if r {
			a.Return = "all"
		} else {
			a.Return = ""
		}
	default:
		return 0, fmt.Errorf("unknown return: %v", r)
	}
	if m.runningArgsMap == nil {
		m.runningArgsMap = make(map[uint16]runningArgs)
	}
	var id uint16
	for {
		id = utils.RandomIDUint16()
		if _, ok := m.runningArgsMap[id]; !ok {
			break
		}
	}
	m.runningArgsMap[id] = a
	return id, nil
}

func (m *MemCache) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (adapter.ReturnMode, error) {
	args := m.runningArgsMap[argsID]
	var ok bool
	switch args.Mode {
	case "store":
		reqMsg := dnsCtx.ReqMsg()
		respMsg := dnsCtx.RespMsg()
		respUpstreamTag := dnsCtx.RespUpstreamTag()
		if reqMsg == nil || respMsg == nil {
			if reqMsg == nil {
				m.logger.DebugContext(ctx, "request message is nil")
				return adapter.ReturnModeContinue, nil
			}
			if respMsg == nil {
				m.logger.DebugContext(ctx, "response message is nil")
				return adapter.ReturnModeContinue, nil
			}
			m.logger.DebugContext(ctx, "request message and response message is nil")
			return adapter.ReturnModeContinue, nil
		}
		key := reqToKey(reqMsg)
		if key == "" {
			m.logger.DebugContext(ctx, "invalid key")
			return adapter.ReturnModeContinue, nil
		}
		ttl := respFindMinTTL(respMsg)
		if ttl == 0 {
			m.logger.DebugContext(ctx, "invalid ttl")
			return adapter.ReturnModeContinue, nil
		}
		cacheMap := m.cacheMap
		if cacheMap != nil {
			cacheMap.Set(key, cacheItem{Upstream: respUpstreamTag, Resp: respMsg.Copy()}, time.Duration(ttl)*time.Second)
			m.logger.Debugf("store key: %s, upstream: %s, ttl: %d", key, respUpstreamTag, ttl)
		}
		ok = true
	case "restore":
		reqMsg := dnsCtx.ReqMsg()
		if reqMsg == nil {
			m.logger.DebugContext(ctx, "request message is nil")
			return adapter.ReturnModeContinue, nil
		}
		key := reqToKey(reqMsg)
		if key == "" {
			m.logger.DebugContext(ctx, "invalid key")
			return adapter.ReturnModeContinue, nil
		}
		cacheMap := m.cacheMap
		if cacheMap != nil {
			cacheItem, isExpired, found := cacheMap.Get(key)
			if found {
				if isExpired {
					m.logger.DebugfContext(ctx, "key: %s is expired", key)
				}
				if !isExpired || m.useExpired {
					m.logger.DebugfContext(ctx, "restore key: %s, upstream: %s", key, cacheItem.Upstream)
					respMsg := cacheItem.Resp.Copy()
					respMsg.SetReply(reqMsg)
					dnsCtx.SetRespMsg(respMsg)
					dnsCtx.SetRespUpstreamTag(cacheItem.Upstream)
				}
				if isExpired && m.flushExpired {
					m.logger.DebugfContext(ctx, "flush key: %s", key)
					go m.flushCache(key, cacheItem.Upstream)
				}
				ok = true
			}
		}
	}
	returnMode := args.Return.(string)
	if ok && returnMode != "" {
		var mode adapter.ReturnMode
		switch returnMode {
		case "all":
			mode = adapter.ReturnModeReturnAll
		case "once":
			mode = adapter.ReturnModeReturnOnce
		}
		m.logger.DebugContext(ctx, mode.String())
		return mode, nil
	}
	return adapter.ReturnModeContinue, nil
}

func (m *MemCache) dumpFileAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.dumpPath == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !m.dumpLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer m.dumpLock.Unlock()
		err := dump(m.cacheMap, m.dumpPath)
		if err != nil {
			m.logger.Errorf("dump cache failed: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (m *MemCache) flushCacheAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cacheMap := m.cacheMap
		if cacheMap != nil {
			cacheMap.FlushAll()
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (m *MemCache) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/dump",
		Methods:     []string{http.MethodGet},
		Description: "dump cache to file if dump-file is set",
		Handler:     m.dumpFileAPIHandler(),
	})
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/flush",
		Methods:     []string{http.MethodGet, http.MethodDelete},
		Description: "flush all cache in memory",
		Handler:     m.flushCacheAPIHandler(),
	})
	return builder.Build()
}

func (m *MemCache) flushCache(key string, upstreamTag string) {
	if upstreamTag == "" {
		m.logger.Error("invalid upstream tag")
		return
	}
	u := m.core.GetUpstream(upstreamTag)
	if u == nil {
		m.logger.Errorf("upstream [%s] not found", upstreamTag)
		return
	}
	req, err := keyToReq(key)
	if err != nil {
		m.logger.Error(err)
		return
	}
	resp, err := u.Exchange(m.ctx, req)
	if err != nil {
		m.logger.Errorf("exchange failed: %s, key: %s", err, key)
		return
	}
	ttl := respFindMinTTL(resp)
	if ttl == 0 {
		m.logger.Errorf("invalid ttl, key: %s", key)
		return
	}
	cacheMap := m.cacheMap
	if cacheMap == nil {
		m.logger.Error("cache map is nil")
		return
	}
	cacheMap.Set(key, cacheItem{Upstream: upstreamTag, Resp: resp}, time.Duration(ttl)*time.Second)
	m.logger.Debugf("flush key: %s, upstream: %s, ttl: %d", key, upstreamTag, ttl)
}

func dump(cacheMap *CacheMap[cacheItem], path string) error {
	if cacheMap == nil {
		return nil
	}
	raw, err := cacheMap.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func reqToKey(req *dns.Msg) string {
	question := req.Question
	var key string
	if len(question) > 0 {
		key = fmt.Sprintf("%s,%s,%s", question[0].Name, dns.TypeToString[question[0].Qtype], dns.ClassToString[question[0].Qclass])
	}
	return key
}

func keyToReq(key string) (*dns.Msg, error) {
	keys := strings.SplitN(key, ",", 3)
	if len(keys) != 3 {
		return nil, fmt.Errorf("invalid key: %s", key)
	}
	name := keys[0]
	qTypeStr := keys[1]
	qClassStr := keys[2]
	qType, ok := dns.StringToType[qTypeStr]
	if !ok {
		return nil, fmt.Errorf("invalid key: %s", key)
	}
	qClass, ok := dns.StringToClass[qClassStr]
	if !ok {
		return nil, fmt.Errorf("invalid key: %s", key)
	}
	req := &dns.Msg{}
	req.SetQuestion(name, qType)
	req.Question[0].Qclass = qClass
	return req, nil
}

func respFindMinTTL(resp *dns.Msg) uint32 {
	var minTTL uint32
	for _, rr := range resp.Answer {
		ttl := rr.Header().Ttl
		if minTTL == 0 || (ttl != 0 && ttl < minTTL) {
			minTTL = ttl
		}
	}
	for _, rr := range resp.Ns {
		ttl := rr.Header().Ttl
		if minTTL == 0 || (ttl != 0 && ttl < minTTL) {
			minTTL = ttl
		}
	}
	for _, rr := range resp.Extra {
		ttl := rr.Header().Ttl
		if minTTL == 0 || (ttl != 0 && ttl < minTTL) {
			minTTL = ttl
		}
	}
	return minTTL
}

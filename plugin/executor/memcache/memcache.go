package memcache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	DumpPath     string         `json:"dump-path"`
	DumpInterval utils.Duration `json:"dump-interval"`
}

type runningArgs struct {
	Mode   string `json:"mode"`
	Return any    `json:"return"`
}

var (
	_ adapter.PluginExecutor = (*MemCache)(nil)
	_ adapter.Starter        = (*MemCache)(nil)
	_ adapter.Closer         = (*MemCache)(nil)
	_ adapter.APIHandler     = (*MemCache)(nil)
)

type MemCache struct {
	ctx            context.Context
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16]runningArgs

	dumpPath     string
	dumpInterval time.Duration

	dumpLock       sync.Mutex
	cacheMap       *CacheMap[*cacheItem]
	loopDumpCtx    context.Context
	loopDumpCancel context.CancelFunc
	closeDone      chan struct{}
}

func NewMemCache(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
	m := &MemCache{
		ctx:    ctx,
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
		cacheMap, err := Decode[*cacheItem](m.ctx, raw)
		if err != nil {
			return fmt.Errorf("load dump file failed: %s, error: %s", m.dumpPath, err)
		}
		m.cacheMap = cacheMap
	} else {
		m.cacheMap = NewCacheMap[*cacheItem](m.ctx)
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
	if a.Return != nil {
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
			cacheMap.Set(key, (*cacheItem)(respMsg.Copy()), time.Duration(ttl)*time.Second)
			m.logger.DebugfContext(ctx, "store key: %s, ttl: %d", key, ttl)
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
			cacheItem, found := cacheMap.Get(key)
			if found {
				m.logger.DebugfContext(ctx, "restore key: %s", key)
				respMsg := copyMsg((*dns.Msg)(cacheItem))
				respMsg.Id = reqMsg.Id
				dnsCtx.SetRespMsg(respMsg)
				ok = true
			}
		}
	}
	var returnMode string
	if args.Return != nil {
		returnMode = args.Return.(string)
	}
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

func dump(cacheMap *CacheMap[*cacheItem], path string) error {
	if cacheMap == nil {
		return nil
	}
	raw, err := cacheMap.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// from mosdns(https://github.com/IrineSistiana/mosdns), thank for @IrineSistiana
func reqToKey(req *dns.Msg) string {
	if req.Response || req.Opcode != dns.OpcodeQuery || len(req.Question) != 1 {
		return ""
	}
	const (
		adBit = 1 << iota
		cdBit
		doBit
	)

	question := req.Question[0]
	buf := make([]byte, 1+2+1+len(question.Name)) // bits + qtype + qname length + qname
	b := byte(0)
	// RFC 6840 5.7: The AD bit in a query as a signal
	// indicating that the requester understands and is interested in the
	// value of the AD bit in the response.
	if req.AuthenticatedData {
		b = b | adBit
	}
	if req.CheckingDisabled {
		b = b | cdBit
	}
	if opt := req.IsEdns0(); opt != nil && opt.Do() {
		b = b | doBit
	}
	buf[0] = b
	buf[1] = byte(question.Qtype << 8)
	buf[2] = byte(question.Qtype)
	buf[3] = byte(len(question.Name))
	copy(buf[4:], question.Name)
	return utils.BytesToStringUnsafe(buf)
}

// from mosdns(https://github.com/IrineSistiana/mosdns), thank for @IrineSistiana
func copyMsg(req *dns.Msg) *dns.Msg {
	if req == nil {
		return nil
	}

	resp := new(dns.Msg)
	resp.MsgHdr = req.MsgHdr
	resp.Compress = req.Compress

	if len(req.Question) > 0 {
		resp.Question = make([]dns.Question, len(req.Question))
		copy(resp.Question, req.Question)
	}

	lenExtra := len(req.Extra)
	for _, r := range req.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			lenExtra--
		}
	}

	s := make([]dns.RR, len(req.Answer)+len(req.Ns)+lenExtra)
	resp.Answer, s = s[:0:len(req.Answer)], s[len(req.Answer):]
	resp.Ns, s = s[:0:len(req.Ns)], s[len(req.Ns):]
	resp.Extra = s[:0:lenExtra]

	for _, r := range req.Answer {
		resp.Answer = append(resp.Answer, dns.Copy(r))
	}
	for _, r := range req.Ns {
		resp.Ns = append(resp.Ns, dns.Copy(r))
	}

	for _, r := range req.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			continue
		}
		resp.Extra = append(resp.Extra, dns.Copy(r))
	}
	return resp
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

type cacheItem dns.Msg

func (c *cacheItem) UnmarshalJSON(data []byte) error {
	var _c string
	err := json.Unmarshal(data, &_c)
	if err != nil {
		return err
	}
	respRaw, err := base64.StdEncoding.DecodeString(_c)
	if err != nil {
		return err
	}
	err = (*dns.Msg)(c).Unpack(respRaw)
	if err != nil {
		return err
	}
	return nil
}

func (c *cacheItem) MarshalJSON() ([]byte, error) {
	respRaw, err := (*dns.Msg)(c).Pack()
	if err != nil {
		return nil, err
	}
	s := base64.StdEncoding.EncodeToString(respRaw)
	return json.Marshal(s)
}

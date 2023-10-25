package rediscache

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
	"github.com/redis/go-redis/v9"
)

const Type = "rediscache"

func init() {
	plugin.RegisterPluginExecutor(Type, NewRedisCache)
}

type Args struct {
	Address  string `json:"address"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type runningArgs struct {
	Mode   string `json:"mode"`
	Return any    `json:"return"`
}

var (
	_ adapter.PluginExecutor = (*RedisCache)(nil)
	_ adapter.Starter        = (*RedisCache)(nil)
	_ adapter.Closer         = (*RedisCache)(nil)
	_ adapter.APIHandler     = (*RedisCache)(nil)
)

type RedisCache struct {
	ctx            context.Context
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16]runningArgs

	address  string
	password string
	db       int

	client *redis.Client
}

func NewRedisCache(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
	r := &RedisCache{
		ctx:    ctx,
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	if a.Address == "" {
		return nil, fmt.Errorf("missing address")
	}
	r.address = a.Address
	r.password = a.Password
	r.db = a.DB
	return r, nil
}

func (r *RedisCache) Tag() string {
	return r.tag
}

func (r *RedisCache) Type() string {
	return Type
}

func (r *RedisCache) Start() error {
	var (
		address = r.address
		network = "unix"
	)
	addr, err := netip.ParseAddrPort(r.address)
	if err != nil {
		network = "tcp"
		address = addr.String()
	}
	r.client = redis.NewClient(&redis.Options{
		Addr:     address,
		Network:  network,
		Password: r.password,
		DB:       r.db,
	})
	return nil
}

func (r *RedisCache) Close() error {
	r.client.Close()
	return nil
}

func (r *RedisCache) LoadRunningArgs(_ context.Context, args any) (uint16, error) {
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
	if r.runningArgsMap == nil {
		r.runningArgsMap = make(map[uint16]runningArgs)
	}
	var id uint16
	for {
		id = utils.RandomIDUint16()
		if _, ok := r.runningArgsMap[id]; !ok {
			break
		}
	}
	r.runningArgsMap[id] = a
	return id, nil
}

func (r *RedisCache) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (adapter.ReturnMode, error) {
	args := r.runningArgsMap[argsID]
	var ok bool
	switch args.Mode {
	case "store":
		reqMsg := dnsCtx.ReqMsg()
		respMsg := dnsCtx.RespMsg()
		if reqMsg == nil || respMsg == nil {
			if reqMsg == nil {
				r.logger.DebugContext(ctx, "request message is nil")
				return adapter.ReturnModeContinue, nil
			}
			if respMsg == nil {
				r.logger.DebugContext(ctx, "response message is nil")
				return adapter.ReturnModeContinue, nil
			}
			r.logger.DebugContext(ctx, "request message and response message is nil")
			return adapter.ReturnModeContinue, nil
		}
		key := reqToKey(reqMsg)
		if key == "" {
			r.logger.DebugContext(ctx, "invalid key")
			return adapter.ReturnModeContinue, nil
		}
		ttl := respFindMinTTL(respMsg)
		if ttl == 0 {
			r.logger.DebugContext(ctx, "invalid ttl")
			return adapter.ReturnModeContinue, nil
		}
		respRaw, err := respMsg.Pack()
		if err != nil {
			r.logger.DebugContext(ctx, "pack response message failed: %w", err)
			return adapter.ReturnModeContinue, nil
		}
		respStr := base64.StdEncoding.EncodeToString(respRaw)
		r.logger.DebugfContext(ctx, "store key: %s, ttl: %d", key, ttl)
		err = r.client.Set(r.ctx, key, respStr, time.Duration(ttl)*time.Second).Err()
		if err != nil {
			r.logger.DebugContext(ctx, "store key failed: %s, error: %w", key, err)
			return adapter.ReturnModeContinue, nil
		}
		ok = true
	case "restore":
		reqMsg := dnsCtx.ReqMsg()
		if reqMsg == nil {
			r.logger.DebugContext(ctx, "request message is nil")
			return adapter.ReturnModeContinue, nil
		}
		key := reqToKey(reqMsg)
		if key == "" {
			r.logger.DebugContext(ctx, "invalid key")
			return adapter.ReturnModeContinue, nil
		}
		value, err := r.client.Get(r.ctx, key).Result()
		if err != nil {
			r.logger.DebugContext(ctx, "get key failed: %s, error: %w", key, err)
			return adapter.ReturnModeContinue, nil
		}
		respRaw, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			r.logger.DebugContext(ctx, "decode response message failed: %w", err)
			return adapter.ReturnModeContinue, nil
		}
		respMsg := &dns.Msg{}
		err = respMsg.Unpack(respRaw)
		if err != nil {
			r.logger.DebugContext(ctx, "unpack response message failed: %w", err)
			return adapter.ReturnModeContinue, nil
		}
		r.logger.DebugfContext(ctx, "restore key: %s", key)
		respMsg.SetReply(reqMsg)
		dnsCtx.SetRespMsg(respMsg)
		ok = true
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
		r.logger.DebugContext(ctx, mode.String())
		return mode, nil
	}
	return adapter.ReturnModeContinue, nil
}

func (r *RedisCache) flushCacheAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.client.FlushAll(req.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

func (r *RedisCache) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/flush",
		Methods:     []string{http.MethodGet, http.MethodDelete},
		Description: "flush all redis cache",
		Handler:     r.flushCacheAPIHandler(),
	})
	return builder.Build()
}

func reqToKey(req *dns.Msg) string {
	question := req.Question
	var key string
	if len(question) > 0 {
		key = fmt.Sprintf("%s,%s,%s", question[0].Name, dns.TypeToString[question[0].Qtype], dns.ClassToString[question[0].Qclass])
	}
	return key
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

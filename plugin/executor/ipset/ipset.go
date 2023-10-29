package ipset

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/plugin/executor/ipset/internal"
	"github.com/rnetx/cdns/utils"

	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
)

const Type = "ipset"

func init() {
	plugin.RegisterPluginExecutor(Type, NewIPSet)
}

const (
	DefaultMask4 = 32
	DefaultMask6 = 128
	DefaultTTL4  = 10 * time.Minute
	DefaultTTL6  = 10 * time.Minute
)

type Args struct {
	Name4    string         `json:"name4"`
	Name6    string         `json:"name6"`
	Create4  bool           `json:"create4"`
	Create6  bool           `json:"create6"`
	Destroy4 bool           `json:"destroy4"`
	Destroy6 bool           `json:"destroy6"`
	Mask4    uint8          `json:"mask4"`
	Mask6    uint8          `json:"mask6"`
	TTL4     utils.Duration `json:"ttl4"`
	TTL6     utils.Duration `json:"ttl6"`
}

type runningArgs struct {
	UseClientIP bool `json:"use-client-ip"`
}

var (
	_ adapter.PluginExecutor = (*IPSet)(nil)
	_ adapter.Starter        = (*IPSet)(nil)
	_ adapter.Closer         = (*IPSet)(nil)
	_ adapter.APIHandler     = (*IPSet)(nil)
)

type IPSet struct {
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16]runningArgs

	name4    string
	name6    string
	create4  bool
	create6  bool
	destroy4 bool
	destroy6 bool
	mask4    uint8
	mask6    uint8
	ttl4     time.Duration
	ttl6     time.Duration

	ipset     internal.IPSet
	flushLock sync.Mutex
}

func NewIPSet(_ context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
	i := &IPSet{
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	if a.Name4 != "" {
		i.name4 = a.Name4
		if a.Mask4 == 0 {
			i.mask4 = DefaultMask4
		} else if a.Mask4 > 32 {
			return nil, fmt.Errorf("invalid mask4: %d", a.Mask4)
		} else {
			i.mask4 = a.Mask4
		}
		if a.TTL4 <= 0 {
			i.ttl4 = DefaultTTL4
		} else {
			i.ttl4 = time.Duration(a.TTL4)
		}
		i.create4 = a.Create4
		i.destroy4 = a.Destroy4
	}
	if a.Name6 != "" {
		i.name6 = a.Name6
		if a.Mask6 == 0 {
			i.mask6 = DefaultMask6
		} else if a.Mask6 > 128 {
			return nil, fmt.Errorf("invalid mask6: %d", a.Mask6)
		} else {
			i.mask6 = a.Mask6
		}
		if a.TTL6 <= 0 {
			i.ttl6 = DefaultTTL6
		} else {
			i.ttl6 = time.Duration(a.TTL6)
		}
		i.create6 = a.Create6
		i.destroy6 = a.Destroy6
	}
	if a.Name4 == "" && a.Name6 == "" {
		return nil, fmt.Errorf("at least one of name4 and name6 must be set")
	}
	return i, nil
}

func (i *IPSet) Tag() string {
	return i.tag
}

func (i *IPSet) Type() string {
	return Type
}

func (i *IPSet) Start() error {
	ipset, err := internal.New()
	if err != nil {
		return fmt.Errorf("init ipset failed: %w", err)
	}
	i.ipset = ipset
	if i.name4 != "" && i.create4 {
		err = ipset.Create(i.name4, i.ttl4)
		if err != nil {
			return fmt.Errorf("create ipset4 failed: %s, error: %w", i.name4, err)
		}
	}
	if i.name6 != "" && i.create6 {
		err = ipset.Create(i.name6, i.ttl6)
		if err != nil {
			return fmt.Errorf("create ipset6 failed: %s, error: %w", i.name6, err)
		}
	}
	return nil
}

func (i *IPSet) Close() error {
	if i.name4 != "" && i.destroy4 {
		err := i.ipset.Destroy(i.name4)
		if err != nil {
			return fmt.Errorf("destroy ipset4 failed: %s, error: %w", i.name4, err)
		}
	}
	if i.name6 != "" && i.destroy6 {
		err := i.ipset.Destroy(i.name6)
		if err != nil {
			return fmt.Errorf("destroy ipset6 failed: %s, error: %w", i.name6, err)
		}
	}
	i.ipset.Close()
	return nil
}

func (i *IPSet) LoadRunningArgs(_ context.Context, args any) (uint16, error) {
	var a runningArgs
	if args != nil {
		err := utils.JsonDecode(args, &a)
		if err != nil {
			return 0, fmt.Errorf("parse args failed: %w", err)
		}
	}
	if i.runningArgsMap == nil {
		i.runningArgsMap = make(map[uint16]runningArgs)
	}
	i.runningArgsMap[0] = runningArgs{}
	var id uint16
	for {
		id = utils.RandomIDUint16()
		if _, ok := i.runningArgsMap[id]; !ok {
			break
		}
	}
	return id, nil
}

func (i *IPSet) addIP(ctx context.Context, addr netip.Addr, ttl uint32) error {
	if addr.Is4() && i.name4 != "" {
		if i.mask4 != 32 {
			prefix := netip.PrefixFrom(addr, int(i.mask4)).Masked()
			ttl := time.Duration(ttl) / time.Second
			if ttl == 0 {
				ttl = i.ttl4
			}
			err := i.ipset.AddPrefix(i.name4, prefix, ttl)
			if err != nil {
				err = fmt.Errorf("add ipset4 failed: %s, prefix: %s, error: %w", i.name4, prefix.String(), err)
				i.logger.ErrorContext(ctx, err)
				return err
			}
			i.logger.DebugfContext(ctx, "add ipset4 success: %s, prefix: %s", i.name4, prefix.String())
		} else {
			ttl := time.Duration(ttl) / time.Second
			if ttl == 0 {
				ttl = i.ttl4
			}
			err := i.ipset.AddAddr(i.name4, addr, ttl)
			if err != nil {
				err = fmt.Errorf("add ipset4 failed: %s, ip: %s, error: %w", i.name4, addr.String(), err)
				i.logger.ErrorContext(ctx, err)
				return err
			}
			i.logger.DebugfContext(ctx, "add ipset4 success: %s, ip: %s", i.name4, addr.String())
		}
	}
	if addr.Is6() && i.name6 != "" {
		if i.mask6 != 128 {
			prefix := netip.PrefixFrom(addr, int(i.mask6)).Masked()
			ttl := time.Duration(ttl) / time.Second
			if ttl == 0 {
				ttl = i.ttl6
			}
			err := i.ipset.AddPrefix(i.name6, prefix, ttl)
			if err != nil {
				err = fmt.Errorf("add ipset6 failed: %s, prefix: %s, error: %w", i.name6, prefix.String(), err)
				i.logger.ErrorContext(ctx, err)
				return err
			}
			i.logger.DebugfContext(ctx, "add ipset6 success: %s, prefix: %s", i.name6, prefix.String())
		} else {
			ttl := time.Duration(ttl) / time.Second
			if ttl == 0 {
				ttl = i.ttl6
			}
			err := i.ipset.AddAddr(i.name6, addr, ttl)
			if err != nil {
				err = fmt.Errorf("add ipset6 failed: %s, ip: %s, error: %w", i.name6, addr.String(), err)
				i.logger.ErrorContext(ctx, err)
				return err
			}
			i.logger.DebugfContext(ctx, "add ipset6 success: %s, ip: %s", i.name6, addr.String())
		}
	}
	return nil
}

func (i *IPSet) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (adapter.ReturnMode, error) {
	args := i.runningArgsMap[argsID]
	if args.UseClientIP {
		clientIP := dnsCtx.ClientIP()
		err := i.addIP(ctx, clientIP, 0)
		if err != nil {
			return adapter.ReturnModeUnknown, err
		}
		return adapter.ReturnModeContinue, nil
	}
	respMsg := dnsCtx.RespMsg()
	if respMsg == nil {
		return adapter.ReturnModeContinue, nil
	}
	ips := make([]netip.Addr, 0, len(respMsg.Answer))
	ttls := make([]uint32, 0, len(respMsg.Answer))
	for _, rr := range respMsg.Answer {
		switch ans := rr.(type) {
		case *dns.A:
			ip, ok := netip.AddrFromSlice(ans.A)
			if ok {
				ips = append(ips, ip)
				ttls = append(ttls, ans.Header().Ttl)
			}
		case *dns.AAAA:
			ip, ok := netip.AddrFromSlice(ans.AAAA)
			if ok {
				ips = append(ips, ip)
				ttls = append(ttls, ans.Header().Ttl)
			}
		}
	}
	var err error
	for index, ip := range ips {
		err = i.addIP(ctx, ip, ttls[index])
		if err != nil {
			return adapter.ReturnModeUnknown, err
		}
	}
	return adapter.ReturnModeContinue, nil
}

func (i *IPSet) flushHandle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !i.flushLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer i.flushLock.Unlock()

		if i.name4 != "" {
			err := i.ipset.Flushall(i.name4)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		if i.name6 != "" {
			err := i.ipset.Flushall(i.name6)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (i *IPSet) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/flush",
		Methods:     []string{http.MethodGet},
		Description: "flush all ipset",
		Handler:     i.flushHandle(),
	})
	return builder.Build()
}

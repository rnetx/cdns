package hosts

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/netip"
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

const Type = "hosts"

func init() {
	plugin.RegisterPluginExecutor(Type, NewHosts)
}

const DefaultTTL = 600 * time.Second

type Args struct {
	File utils.Listable[string] `json:"file"`
	Rule utils.Listable[string] `json:"rule"`
	TTL  utils.Duration         `json:"ttl"`
}

var (
	_ adapter.PluginExecutor = (*Hosts)(nil)
	_ adapter.Starter        = (*Hosts)(nil)
	_ adapter.APIHandler     = (*Hosts)(nil)
)

type Hosts struct {
	ctx    context.Context
	tag    string
	logger log.Logger

	files []string
	rules []string
	ttl   time.Duration

	insideRuleMap map[string][]netip.Addr
	fileRuleMap   map[string][]netip.Addr

	reloadLock sync.Mutex
}

func NewHosts(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
	h := &Hosts{
		ctx:    ctx,
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	if len(a.File) == 0 && len(a.Rule) == 0 {
		return nil, fmt.Errorf("empty args")
	}
	h.files = a.File
	h.rules = a.Rule
	if a.TTL > 0 {
		h.ttl = time.Duration(a.TTL)
	} else {
		h.ttl = DefaultTTL
	}
	return h, nil
}

func (h *Hosts) Tag() string {
	return h.tag
}

func (h *Hosts) Type() string {
	return Type
}

func (h *Hosts) Start() error {
	if len(h.rules) > 0 {
		insideRuleMap, err := decodeRules(h.rules)
		if err != nil {
			return fmt.Errorf("decode rules failed: %w", err)
		}
		h.insideRuleMap = insideRuleMap
		h.logger.Infof("load rules success: %d", len(insideRuleMap))
	}
	if len(h.files) > 0 {
		m, err := loadFiles(h.files)
		if err != nil {
			return err
		}
		h.fileRuleMap = m
		h.logger.Infof("load files success: %d", len(m))
	}
	return nil
}

func (h *Hosts) LoadRunningArgs(_ context.Context, _ any) (uint16, error) {
	return 0, nil
}

func (h *Hosts) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, _ uint16) (adapter.ReturnMode, error) {
	reqMsg := dnsCtx.ReqMsg()
	if reqMsg == nil {
		h.logger.DebugContext(ctx, "request message is nil")
		return adapter.ReturnModeContinue, nil
	}
	question := reqMsg.Question
	if len(question) == 0 {
		h.logger.DebugContext(ctx, "request question is empty")
		return adapter.ReturnModeContinue, nil
	}
	name := question[0].Name
	qType := question[0].Qtype
	if qType != dns.TypeA && qType != dns.TypeAAAA {
		h.logger.DebugContext(ctx, "request type is not A or AAAA")
		return adapter.ReturnModeContinue, nil
	}

	r1 := h.insideRuleMap[name]
	r2 := h.fileRuleMap[name]
	r := mergeSlice(r1, r2)

	answers := make([]dns.RR, 0, len(r))
	for _, ip := range r {
		if ip.Is4() && qType == dns.TypeA {
			answers = append(answers, &dns.A{
				Hdr: dns.RR_Header{
					Name:   name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    uint32(h.ttl.Seconds()),
				},
				A: ip.AsSlice(),
			})
		}
		if ip.Is6() && qType == dns.TypeAAAA {
			answers = append(answers, &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    uint32(h.ttl.Seconds()),
				},
				AAAA: ip.AsSlice(),
			})
		}
	}
	respMsg := &dns.Msg{}
	respMsg.SetReply(reqMsg)
	respMsg.Answer = answers

	return adapter.ReturnModeContinue, nil
}

func (h *Hosts) reloadFileRules() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.reloadLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer h.reloadLock.Unlock()
		h.logger.Infof("reload file rules")
		fileRuleMap, err := loadFiles(h.files)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			h.logger.Infof("reload file rules success: %d", len(fileRuleMap))
			h.fileRuleMap = fileRuleMap
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (h *Hosts) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/reload",
		Methods:     []string{http.MethodGet},
		Description: "reload file rules if file is set",
		Handler:     h.reloadFileRules(),
	})
	return builder.Build()
}

func decodeRules(rules []string) (map[string][]netip.Addr, error) {
	m := make(map[string][]netip.Addr)
	for _, rule := range rules {
		if strings.HasPrefix(rule, "#") {
			continue
		}
		rule = strings.TrimPrefix(rule, " ")
		if rule == "" {
			continue
		}
		s := strings.SplitAfterN(rule, "#", 2)
		if len(s) == 2 {
			rule = strings.TrimSuffix(s[0], "#")
			rule = strings.TrimSuffix(rule, " ")
		}
		ss := strings.Split(rule, " ")
		if len(ss) < 2 {
			continue
		}
		domain := ss[0]
		ips := make([]netip.Addr, 0, len(ss)-1)
		for _, ip := range ss[1:] {
			a, err := netip.ParseAddr(ip)
			if err != nil {
				return nil, fmt.Errorf("invalid rule: %s", rule)
			}
			ips = append(ips, a)
		}
		domain = dns.Fqdn(domain)
		oldRules, ok := m[domain]
		if !ok {
			m[domain] = ips
		} else {
			m[domain] = mergeSlice(oldRules, ips)
		}
	}
	return m, nil
}

func mergeSlice[T fmt.Stringer](old, new []T) []T {
	m := make(map[string]T)
	for _, v := range old {
		m[v.String()] = v
	}
	for _, v := range new {
		m[v.String()] = v
	}
	r := make([]T, 0, len(m))
	for _, v := range m {
		r = append(r, v)
	}
	return r
}

func loadFile(file string) (map[string][]netip.Addr, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}

	return decodeRules(lines)
}

func loadFiles(files []string) (map[string][]netip.Addr, error) {
	var rr map[string][]netip.Addr
	for _, file := range files {
		rules, err := loadFile(file)
		if err != nil {
			return nil, fmt.Errorf("load file failed: %s, error: %w", file, err)
		}
		if rr == nil {
			rr = rules
		} else {
			rr = mergeMap(rr, rules)
		}
	}
	return rr, nil
}

func mergeMap[T fmt.Stringer](old, new map[string][]T) map[string][]T {
	m := make(map[string][]T)
	for k, v := range old {
		m[k] = v
	}
	for k, v := range new {
		oldRules, ok := m[k]
		if !ok {
			m[k] = v
		} else {
			m[k] = mergeSlice(oldRules, v)
		}
	}
	return m
}

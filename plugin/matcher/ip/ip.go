package ip

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
)

const Type = "ip"

func init() {
	plugin.RegisterPluginMatcher(Type, NewIP)
}

type Args struct {
	File utils.Listable[string] `json:"file"`
	Rule utils.Listable[string] `json:"rule"`
}

type IP struct {
	ctx    context.Context
	tag    string
	logger log.Logger

	files []string
	rules []string

	insideRules []netip.Prefix
	fileRules   []netip.Prefix

	reloadLock sync.Mutex
}

func NewIP(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error) {
	i := &IP{
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
	i.files = a.File
	i.rules = a.Rule
	return i, nil
}

func (i *IP) Tag() string {
	return i.tag
}

func (i *IP) Type() string {
	return Type
}

func (i *IP) Start() error {
	if len(i.rules) > 0 {
		insideRules, n, err := decodeRules(i.rules)
		if err != nil {
			return fmt.Errorf("decode rules failed: %w", err)
		}
		i.insideRules = insideRules
		i.logger.Infof("load rules success: %d", n)
	}
	if len(i.files) > 0 {
		sets, n, err := loadFiles(i.files)
		if err != nil {
			return err
		}
		i.fileRules = sets
		i.logger.Infof("load files success: %d", n)
	}
	return nil
}

func (i *IP) LoadRunningArgs(_ context.Context, _ any) (uint16, error) {
	return 0, nil
}

func (i *IP) Match(ctx context.Context, dnsCtx *adapter.DNSContext, _ uint16) (bool, error) {
	respMsg := dnsCtx.RespMsg()
	if respMsg == nil {
		i.logger.DebugfContext(ctx, "response message is nil")
		return false, nil
	}
	if len(respMsg.Answer) == 0 {
		i.logger.DebugfContext(ctx, "response message answer is empty")
		return false, nil
	}
	ips := make([]netip.Addr, 0, len(respMsg.Answer))
	for _, ans := range respMsg.Answer {
		switch r := ans.(type) {
		case *dns.A:
			ip, ok := netip.AddrFromSlice(r.A)
			if ok {
				ips = append(ips, ip)
			}
		case *dns.AAAA:
			ip, ok := netip.AddrFromSlice(r.AAAA)
			if ok {
				ips = append(ips, ip)
			}
		}
	}
	if len(ips) == 0 {
		i.logger.DebugfContext(ctx, "no ips found")
		return false, nil
	}
	if i.insideRules != nil {
		for _, ip := range ips {
			for _, p := range i.insideRules {
				if p.Contains(ip) {
					i.logger.DebugfContext(ctx, "match rule: %s => %s", p.String(), ip.String())
					return true, nil
				}
			}
		}
	}
	fileRules := i.fileRules
	for _, ip := range ips {
		for _, p := range i.insideRules {
			if p.Contains(ip) {
				i.logger.DebugfContext(ctx, "match rule: %s => %s", p.String(), ip.String())
				return true, nil
			}
		}
		for _, p := range fileRules {
			if p.Contains(ip) {
				i.logger.DebugfContext(ctx, "match rule: %s => %s", p.String(), ip.String())
				return true, nil
			}
		}
	}
	return false, nil
}

func (i *IP) reloadFileRules() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !i.reloadLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer i.reloadLock.Unlock()
		i.logger.Infof("reload file rules")
		fileRules, n, err := loadFiles(i.files)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			i.logger.Infof("reload file rules success: %d", n)
			i.fileRules = fileRules
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (i *IP) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/reload",
		Methods:     []string{http.MethodGet},
		Description: "reload file rules if file is set",
		Handler:     i.reloadFileRules(),
	})
	return builder.Build()
}

func decodeRules(rules []string) ([]netip.Prefix, int, error) {
	prefixs := make([]netip.Prefix, 0, len(rules))
	var n int
	prefixMap := make(map[string]bool)
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
		prefix, err := netip.ParsePrefix(rule)
		if err == nil {
			pp := prefix.String()
			if !prefixMap[pp] {
				n++
				prefixMap[pp] = true
				prefixs = append(prefixs, prefix)
			}
			continue
		}
		ip, err := netip.ParseAddr(rule)
		if err == nil {
			bits := 0
			if ip.Is4() {
				bits = 32
			} else {
				bits = 128
			}
			prefix = netip.PrefixFrom(ip, bits)
			pp := prefix.String()
			if !prefixMap[pp] {
				n++
				prefixMap[pp] = true
				prefixs = append(prefixs, prefix)
			}
			continue
		}
		return nil, 0, fmt.Errorf("invalid rule: %s", rule)
	}
	return prefixs, n, nil
}

func loadFile(file string) ([]netip.Prefix, int, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, 0, err
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

func loadFiles(files []string) ([]netip.Prefix, int, error) {
	prefixs := make([]netip.Prefix, 0, len(files))
	var n int
	for _, file := range files {
		rules, nn, err := loadFile(file)
		if err != nil {
			return nil, 0, fmt.Errorf("load file failed: %s, error: %w", file, err)
		}
		prefixs = append(prefixs, rules...)
		n += nn
	}
	prefixMap := make(map[string]bool)
	newPrefix := make([]netip.Prefix, 0, n)
	for _, prefix := range prefixs {
		pp := prefix.String()
		if !prefixMap[pp] {
			prefixMap[pp] = true
			newPrefix = append(newPrefix, prefix)
		} else {
			n--
		}
	}
	return newPrefix, n, nil
}

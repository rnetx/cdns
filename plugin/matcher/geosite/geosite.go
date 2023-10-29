package geosite

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/plugin/matcher/geosite/meta"
	"github.com/rnetx/cdns/plugin/matcher/geosite/sing"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/utils/domain"

	"github.com/go-chi/chi/v5"
)

const Type = "geosite"

func init() {
	plugin.RegisterPluginMatcher(Type, NewGeoSite)
}

type Args struct {
	Path string                 `json:"path"`
	Type string                 `json:"type"`
	Code utils.Listable[string] `json:"code"`
}

type runningArgs struct {
	Code utils.Listable[string] `json:"code"`
}

var (
	_ adapter.PluginMatcher = (*GeoSite)(nil)
	_ adapter.Starter       = (*GeoSite)(nil)
	_ adapter.APIHandler    = (*GeoSite)(nil)
)

type GeoSite struct {
	ctx            context.Context
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16][]string

	path        string
	geositeType string
	code        []string

	rule       *domain.DomainSet
	ruleMap    map[string]*domain.DomainSet
	reloadLock sync.Mutex
}

func NewGeoSite(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error) {
	g := &GeoSite{
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
	switch a.Type {
	case "sing", "sing-box":
		g.geositeType = "sing"
	case "meta", "clash.meta", "clash-meta":
		g.geositeType = "meta"
	case "":
		return nil, fmt.Errorf("missing type")
	default:
		return nil, fmt.Errorf("invalid type: %s", a.Type)
	}
	g.path = a.Path
	g.code = a.Code
	return g, nil
}

func (g *GeoSite) Tag() string {
	return g.tag
}

func (g *GeoSite) Type() string {
	return Type
}

func (g *GeoSite) Start() error {
	return g.loadRule()
}

func (g *GeoSite) loadSingRule() error {
	reader, codes, err := sing.OpenReader(g.path)
	if err != nil {
		return fmt.Errorf("open sing-geosite file failed: %s, errors: %s", g.path, err)
	}
	defer reader.Close()
	var loadCodes []string
	if len(g.code) == 0 {
		loadCodes = codes
	} else {
		loadCodes = make([]string, 0, len(g.code))
		for _, code1 := range codes {
			for _, code2 := range g.code {
				if code1 == code2 {
					loadCodes = append(loadCodes, code1)
					break
				}
			}
		}
	}
	ruleMap := make(map[string]*domain.DomainSet, len(loadCodes))
	for _, code := range loadCodes {
		ss, err := reader.Read(code)
		if err != nil {
			return fmt.Errorf("read sing-geosite file failed: %s, errors: read code: %s", g.path, err)
		}
		ruleMap[code] = ss
	}
	g.ruleMap = ruleMap
	g.logger.Infof("load %d codes", len(g.ruleMap))
	return nil
}

func (g *GeoSite) loadMetaRule() error {
	rule, length, err := meta.ReadFile(g.path)
	if err != nil {
		return fmt.Errorf("read meta-geosite file failed: %s, errors: %s", g.path, err)
	}
	g.rule = rule
	g.logger.Infof("load %d rules", length)
	return nil
}

func (g *GeoSite) loadRule() error {
	switch g.geositeType {
	case "sing":
		return g.loadSingRule()
	case "meta":
		return g.loadMetaRule()
	}
	return nil
}

func (g *GeoSite) LoadRunningArgs(_ context.Context, args any) (uint16, error) {
	switch g.geositeType {
	case "sing":
		var codes utils.Listable[string]
		err := utils.JsonDecode(args, &codes)
		if err != nil {
			var a runningArgs
			err2 := utils.JsonDecode(args, &a)
			if err2 != nil {
				return 0, fmt.Errorf("%w | %w", err, err2)
			}
			codes = a.Code
		}
		if len(codes) == 0 {
			return 0, fmt.Errorf("missing code")
		}
		seen := make(map[string]struct{}, len(codes))
		formatCodes := make([]string, 0, len(codes))
		for _, code := range codes {
			c := strings.Split(code, ",")
			for _, cc := range c {
				cc = strings.TrimSpace(cc)
				if _, ok := seen[cc]; !ok {
					seen[cc] = struct{}{}
					formatCodes = append(formatCodes, cc)
				}
			}
		}
		if g.runningArgsMap == nil {
			g.runningArgsMap = make(map[uint16][]string)
		}
		var id uint16
		for {
			id = utils.RandomIDUint16()
			if _, ok := g.runningArgsMap[id]; !ok {
				break
			}
		}
		g.runningArgsMap[id] = formatCodes
		return id, nil
	case "meta":
	}
	return 0, nil
}

func (g *GeoSite) Match(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (bool, error) {
	reqMsg := dnsCtx.ReqMsg()
	question := reqMsg.Question[0]
	name := question.Name
	switch g.geositeType {
	case "sing":
		codes := g.runningArgsMap[argsID]
		ruleMap := g.ruleMap
		for _, code := range codes {
			set, ok := ruleMap[code]
			if ok {
				if set.Match(name) {
					g.logger.DebugfContext(ctx, "match %s, code: %s", name, code)
					return true, nil
				}
			}
		}
	case "meta":
		rule := g.rule
		if rule.Match(name) {
			g.logger.DebugfContext(ctx, "match %s", name)
			return true, nil
		}
	}
	return false, nil
}

func (g *GeoSite) reloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !g.reloadLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer g.reloadLock.Unlock()
		g.logger.Infof("reload geosite rule...")
		err := g.loadRule()
		if err != nil {
			g.logger.Errorf("reload geosite rule failed: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			g.logger.Infof("reload geosite rule success")
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (g *GeoSite) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/reload",
		Methods:     []string{http.MethodGet},
		Description: "reload geosite rule.",
		Handler:     g.reloadHandler(),
	})
	return builder.Build()
}

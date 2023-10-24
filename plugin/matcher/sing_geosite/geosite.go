package sing_geosite

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/utils/domain"
)

const Type = "sing-geosite"

func init() {
	plugin.RegisterPluginMatcher(Type, NewSingGeoSite)
}

type Args struct {
	Path string                 `json:"path"`
	Code utils.Listable[string] `json:"code"`
}

var (
	_ adapter.PluginMatcher = (*SingGeoSite)(nil)
	_ adapter.Starter       = (*SingGeoSite)(nil)
	_ adapter.APIHandler    = (*SingGeoSite)(nil)
)

type SingGeoSite struct {
	ctx            context.Context
	tag            string
	logger         log.Logger
	runningArgsMap map[uint16][]string

	path string
	code []string

	ruleMap    map[string]*domain.DomainSet
	reloadLock sync.Mutex
}

func NewSingGeoSite(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error) {
	s := &SingGeoSite{
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
	s.path = a.Path
	s.code = a.Code
	return s, nil
}

func (s *SingGeoSite) Tag() string {
	return s.tag
}

func (s *SingGeoSite) Type() string {
	return Type
}

func (s *SingGeoSite) Start() error {
	return s.loadRule()
}

func (s *SingGeoSite) loadRule() error {
	reader, codes, err := OpenGeoSiteReader(s.path)
	if err != nil {
		return fmt.Errorf("open sing-geosite file failed: %s, errors: %s", s.path, err)
	}
	defer reader.Close()
	var loadCodes []string
	if len(s.code) == 0 {
		loadCodes = codes
	} else {
		loadCodes = make([]string, 0, len(s.code))
		for _, code1 := range codes {
			for _, code2 := range s.code {
				if code1 == code2 {
					loadCodes = append(loadCodes, code1)
					break
				}
			}
		}
	}
	ruleMap := make(map[string]*domain.DomainSet, len(loadCodes))
	for _, code := range loadCodes {
		items, err := reader.Read(code)
		if err != nil {
			return fmt.Errorf("read sing-geosite file failed: %s, errors: read code: %s", s.path, err)
		}
		ss, err := compile(items)
		if err != nil {
			return fmt.Errorf("compile sing-geosite file failed: %s, errors: compile rule: %s", s.path, err)
		}
		ruleMap[code] = ss
	}
	s.ruleMap = ruleMap
	s.logger.Infof("load %d codes", len(s.ruleMap))
	return nil
}

func (s *SingGeoSite) LoadRunningArgs(_ context.Context, args any) (uint16, error) {
	var codes utils.Listable[string]
	err := utils.JsonDecode(args, &codes)
	if err != nil {
		return 0, err
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
	if s.runningArgsMap == nil {
		s.runningArgsMap = make(map[uint16][]string)
	}
	var id uint16
	for {
		id = utils.RandomIDUint16()
		if _, ok := s.runningArgsMap[id]; !ok {
			break
		}
	}
	s.runningArgsMap[id] = formatCodes
	return id, nil
}

func (s *SingGeoSite) Match(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (bool, error) {
	reqMsg := dnsCtx.ReqMsg()
	question := reqMsg.Question
	if len(question) == 0 {
		s.logger.DebugfContext(ctx, "request question is empty")
		return false, nil
	}
	name := question[0].Name
	codes := s.runningArgsMap[argsID]
	ruleMap := s.ruleMap
	for _, code := range codes {
		set, ok := ruleMap[code]
		if ok {
			if set.Match(name) {
				s.logger.DebugfContext(ctx, "match %s, code: %s", name, code)
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *SingGeoSite) reloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.reloadLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer s.reloadLock.Unlock()
		s.logger.Infof("reload sing-geosite rule...")
		err := s.loadRule()
		if err != nil {
			s.logger.Errorf("reload sing-geosite rule failed: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			s.logger.Infof("reload sing-geosite rule success")
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (s *SingGeoSite) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/reload",
		Methods:     []string{http.MethodGet},
		Description: "reload sing-geosite rule.",
		Handler:     s.reloadHandler(),
	})
	return builder.Build()
}

package domain

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/utils/domain"

	"github.com/go-chi/chi/v5"
)

const Type = "domain"

func init() {
	plugin.RegisterPluginMatcher(Type, NewDomain)
}

type Args struct {
	File utils.Listable[string] `json:"file"`
	Rule utils.Listable[string] `json:"rule"`
}

var (
	_ adapter.PluginMatcher = (*Domain)(nil)
	_ adapter.Starter       = (*Domain)(nil)
	_ adapter.APIHandler    = (*Domain)(nil)
)

type Domain struct {
	ctx    context.Context
	tag    string
	logger log.Logger

	files []string
	rules []string

	insideRuleSet *domain.DomainSet
	fileRuleSet   []*domain.DomainSet

	reloadLock sync.Mutex
}

func NewDomain(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error) {
	d := &Domain{
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
	d.files = a.File
	d.rules = a.Rule
	return d, nil
}

func (d *Domain) Tag() string {
	return d.tag
}

func (d *Domain) Type() string {
	return Type
}

func (d *Domain) Start() error {
	if len(d.rules) > 0 {
		insideRuleSet, n, err := decodeRules(d.rules)
		if err != nil {
			return fmt.Errorf("decode rules failed: %w", err)
		}
		d.insideRuleSet = insideRuleSet
		d.logger.Infof("load rules success: %d", n)
	}
	if len(d.files) > 0 {
		sets, n, err := loadFiles(d.files)
		if err != nil {
			return err
		}
		d.fileRuleSet = sets
		d.logger.Infof("load files success: %d", n)
	}
	return nil
}

func (d *Domain) LoadRunningArgs(_ context.Context, _ any) (uint16, error) {
	return 0, nil
}

func (d *Domain) Match(ctx context.Context, dnsCtx *adapter.DNSContext, _ uint16) (bool, error) {
	reqMsg := dnsCtx.ReqMsg()
	if reqMsg == nil {
		d.logger.DebugContext(ctx, "request message is nil")
		return false, nil
	}
	question := reqMsg.Question
	if len(question) == 0 {
		d.logger.DebugContext(ctx, "request question is empty")
		return false, nil
	}
	name := question[0].Name
	name = strings.TrimSuffix(name, ".")

	if d.insideRuleSet.Match(name) {
		d.logger.DebugfContext(ctx, "match rule: %s", question[0].Name)
		return true, nil
	}
	fileRules := d.fileRuleSet
	for _, set := range fileRules {
		if set.Match(name) {
			d.logger.DebugfContext(ctx, "match rule: %s", question[0].Name)
			return true, nil
		}
	}
	return false, nil
}

func (d *Domain) reloadFileRules() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !d.reloadLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer d.reloadLock.Unlock()
		d.logger.Infof("reload file rules")
		fileRules, n, err := loadFiles(d.files)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			d.logger.Infof("reload file rules success: %d", n)
			d.fileRuleSet = fileRules
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (d *Domain) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/reload",
		Methods:     []string{http.MethodGet},
		Description: "reload file rules if file is set",
		Handler:     d.reloadFileRules(),
	})
	return builder.Build()
}

func decodeRules(rules []string) (*domain.DomainSet, int, error) {
	builder := domain.NewDomainSetBuilder()
	var n int
	var (
		fullMap    = make(map[string]bool)
		suffixMap  = make(map[string]bool)
		keywordMap = make(map[string]bool)
		regexpMap  = make(map[string]bool)
	)
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
		switch {
		case strings.HasPrefix(rule, "full:"):
			bRule := rule[5:]
			if !fullMap[bRule] {
				fullMap[bRule] = true
				builder.AddFull(bRule)
				n++
			}
		case strings.HasPrefix(rule, "suffix:"):
			bRule := rule[7:]
			if !strings.HasPrefix(bRule, ".") {
				bRule = "." + bRule
			}
			if !suffixMap[bRule] {
				suffixMap[bRule] = true
				builder.AddSuffix(bRule)
				n++
			}
		case strings.HasPrefix(rule, "keyword:"):
			bRule := rule[8:]
			if !keywordMap[bRule] {
				keywordMap[bRule] = true
				builder.AddKeyword(bRule)
				n++
			}
		case strings.HasPrefix(rule, "regexp:"):
			bRule := rule[7:]
			if strings.HasPrefix(bRule, "\"") {
				if !strings.HasSuffix(bRule, "\"") {
					return nil, 0, fmt.Errorf("invalid rule: %s", rule)
				}
				bRule = bRule[1 : len(bRule)-1]
			}
			if !regexpMap[bRule] {
				regexpMap[bRule] = true
				builder.AddRegexp(bRule)
				n++
			}
		case strings.HasPrefix(rule, "regex:"):
			bRule := rule[6:]
			if strings.HasPrefix(bRule, "\"") {
				if !strings.HasSuffix(bRule, "\"") {
					return nil, 0, fmt.Errorf("invalid rule: %s", rule)
				}
				bRule = bRule[1 : len(bRule)-1]
			}
			if !regexpMap[bRule] {
				regexpMap[bRule] = true
				builder.AddRegexp(bRule)
				n++
			}
		case strings.Contains(rule, ":"):
			return nil, 0, fmt.Errorf("invalid rule: %s", rule)
		default:
			bRule := rule
			if !strings.HasPrefix(bRule, ".") {
				bRule = "." + bRule
				if !fullMap[rule] {
					fullMap[rule] = true
					builder.AddFull(rule)
					n++
				}
			}
			if !suffixMap[bRule] {
				suffixMap[bRule] = true
				builder.AddSuffix(bRule)
				n++
			}
		}
	}
	set, err := builder.Build()
	if err != nil {
		return nil, 0, err
	}
	return set, n, nil
}

func loadFile(file string) (*domain.DomainSet, int, error) {
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

func loadFiles(files []string) ([]*domain.DomainSet, int, error) {
	sets := make([]*domain.DomainSet, 0, len(files))
	var n int
	for _, file := range files {
		set, nn, err := loadFile(file)
		if err != nil {
			return nil, 0, fmt.Errorf("load file failed: %s, error: %w", file, err)
		}
		sets = append(sets, set)
		n += nn
	}
	return sets, n, nil
}

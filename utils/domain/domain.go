package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type DomainSetBuilder struct {
	fulls    []string // full
	suffixs  []string // suffix
	keywords []string // keyword
	regexps  []string // regexp
}

func NewDomainSetBuilder() *DomainSetBuilder {
	return &DomainSetBuilder{}
}

func NewDomainSetBuilderWithSize(full, suffix, keyword, regexp int) *DomainSetBuilder {
	return &DomainSetBuilder{
		fulls:    make([]string, 0, full),
		suffixs:  make([]string, 0, suffix),
		keywords: make([]string, 0, keyword),
		regexps:  make([]string, 0, regexp),
	}
}

func (b *DomainSetBuilder) Len() int {
	return len(b.fulls) + len(b.suffixs) + len(b.keywords) + len(b.regexps)
}

func (b *DomainSetBuilder) AddFull(full string) {
	b.fulls = append(b.fulls, full)
}

func (b *DomainSetBuilder) AddSuffix(suffix string) {
	b.suffixs = append(b.suffixs, suffix)
}

func (b *DomainSetBuilder) AddKeyword(keyword string) {
	b.keywords = append(b.keywords, keyword)
}

func (b *DomainSetBuilder) AddRegexp(regexp string) {
	b.regexps = append(b.regexps, regexp)
}

type DomainSet struct {
	trie     *succinctSet     // full && suffix
	keywords []string         // keyword
	regexps  []*regexp.Regexp // regex
}

func (b *DomainSetBuilder) Build() (*DomainSet, error) {
	s := &DomainSet{}
	if len(b.keywords) > 0 {
		s.keywords = b.keywords
	}
	if len(b.regexps) > 0 {
		s.regexps = make([]*regexp.Regexp, len(b.regexps))
		for i, r := range b.regexps {
			regex, err := regexp.Compile(r)
			if err != nil {
				return nil, fmt.Errorf("compile regexp %s failed: %v", r, err)
			}
			s.regexps[i] = regex
		}
	}
	if len(b.fulls) > 0 || len(b.suffixs) > 0 {
		domainList := make([]string, 0, len(b.fulls)+len(b.suffixs))
		seen := make(map[string]bool, len(domainList))
		for _, domain := range b.suffixs {
			if seen[domain] {
				continue
			}
			seen[domain] = true
			domainList = append(domainList, reverseDomainSuffix(domain))
		}
		for _, domain := range b.fulls {
			if seen[domain] {
				continue
			}
			seen[domain] = true
			domainList = append(domainList, reverseDomain(domain))
		}
		sort.Strings(domainList)
		s.trie = newSuccinctSet(domainList)
	}
	return s, nil
}

func reverseDomain(domain string) string {
	l := len(domain)
	b := make([]byte, l)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	return string(b)
}

func reverseDomainSuffix(domain string) string {
	l := len(domain)
	b := make([]byte, l+1)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	b[l] = prefixLabel
	return string(b)
}

func (d *DomainSet) Match(domain string) bool {
	domain = strings.TrimSuffix(domain, ".")
	if d.trie != nil {
		if d.trie.Has(reverseDomain(domain)) {
			return true
		}
	}
	if d.keywords != nil {
		for _, keyword := range d.keywords {
			if strings.Contains(domain, keyword) {
				return true
			}
		}
	}
	if d.regexps != nil {
		for _, regex := range d.regexps {
			if regex.MatchString(domain) {
				return true
			}
		}
	}
	return false
}

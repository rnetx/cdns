package sing

import (
	"strings"

	"github.com/rnetx/cdns/utils/domain"
)

type itemType = uint8

const (
	RuleTypeDomain itemType = iota
	RuleTypeDomainSuffix
	RuleTypeDomainKeyword
	RuleTypeDomainRegex
)

type item struct {
	Type  itemType
	Value string
}

func compile(code []item) (*domain.DomainSet, error) {
	var domainLength int
	var domainSuffixLength int
	var domainKeywordLength int
	var domainRegexLength int
	for _, item := range code {
		switch item.Type {
		case RuleTypeDomain:
			domainLength++
		case RuleTypeDomainSuffix:
			domainSuffixLength++
		case RuleTypeDomainKeyword:
			domainKeywordLength++
		case RuleTypeDomainRegex:
			domainRegexLength++
		}
	}
	var (
		full    = 0
		suffix  = 0
		keyword = 0
		regexp  = 0
	)
	if domainLength > 0 {
		full = domainLength
	}
	if domainSuffixLength > 0 {
		suffix = domainSuffixLength
	}
	if domainKeywordLength > 0 {
		keyword = domainKeywordLength
	}
	if domainRegexLength > 0 {
		regexp = domainRegexLength
	}
	builder := domain.NewDomainSetBuilderWithSize(full, suffix, keyword, regexp)
	for _, item := range code {
		switch item.Type {
		case RuleTypeDomain:
			builder.AddFull(item.Value)
		case RuleTypeDomainSuffix:
			value := item.Value
			if !strings.HasPrefix(value, ".") {
				value = "." + value
			}
			builder.AddSuffix(value)
		case RuleTypeDomainKeyword:
			builder.AddKeyword(item.Value)
		case RuleTypeDomainRegex:
			builder.AddRegexp(item.Value)
		}
	}
	return builder.Build()
}

package sing_geosite

import "github.com/rnetx/cdns/utils/domain"

type ItemType = uint8

const (
	RuleTypeDomain ItemType = iota
	RuleTypeDomainSuffix
	RuleTypeDomainKeyword
	RuleTypeDomainRegex
)

type Item struct {
	Type  ItemType
	Value string
}

func compile(code []Item) (*domain.DomainSet, error) {
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
	builder := domain.NewDomainSetBuildWithSize(full, suffix, keyword, regexp)
	for _, item := range code {
		switch item.Type {
		case RuleTypeDomain:
			builder.AddFull(item.Value)
		case RuleTypeDomainSuffix:
			builder.AddSuffix(item.Value)
		case RuleTypeDomainKeyword:
			builder.AddKeyword(item.Value)
		case RuleTypeDomainRegex:
			builder.AddRegexp(item.Value)
		}
	}
	return builder.Build()
}

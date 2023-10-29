package v2xray

import (
	"fmt"
	"os"
	"strings"

	"github.com/rnetx/cdns/utils/domain"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

func ReadRule(path string, codes []string) (map[string]*domain.DomainSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var vGeositeList routercommon.GeoSiteList
	err = proto.Unmarshal(raw, &vGeositeList)
	if err != nil {
		return nil, err
	}

	var loadAll bool
	if len(codes) == 0 {
		loadAll = true
	}
	codeMap := make(map[string]struct{})
	mainCodeMap := make(map[string]struct{})
	for _, code := range codes {
		code = strings.ToLower(code)
		cc := strings.SplitN(code, "@", 2)
		if len(cc) != 2 {
			mainCodeMap[code] = struct{}{}
			codeMap[code] = struct{}{}
		} else {
			mainCodeMap[cc[0]] = struct{}{}
			codeMap[code] = struct{}{}
		}
	}

	m := make(map[string]*domain.DomainSetBuilder)
	for _, vGeositeEntry := range vGeositeList.Entry {
		code := strings.ToLower(vGeositeEntry.CountryCode)
		if _, ok := mainCodeMap[code]; !ok && !loadAll {
			continue
		}
		builder := domain.NewDomainSetBuilder()
		setMap := make(map[string]bool, len(vGeositeEntry.Domain))
		attributes := make(map[string][]*routercommon.Domain)
		for _, domain := range vGeositeEntry.Domain {
			if len(domain.Attribute) > 0 {
				for _, attribute := range domain.Attribute {
					k := strings.ToLower(code + "@" + attribute.Key)
					if _, ok := codeMap[k]; !ok && !loadAll {
						continue
					}
					attributes[k] = append(attributes[k], domain)
				}
			}
			if _, ok := codeMap[code]; !ok && !loadAll {
				continue
			}
			switch domain.Type {
			case routercommon.Domain_Full:
				if _, ok := setMap["full:"+domain.Value]; !ok {
					builder.AddFull(domain.Value)
					setMap["full:"+domain.Value] = true
				}
			case routercommon.Domain_Plain:
				if _, ok := setMap["keyword:"+domain.Value]; !ok {
					builder.AddKeyword(domain.Value)
					setMap["keyword:"+domain.Value] = true
				}
			case routercommon.Domain_Regex:
				if _, ok := setMap["regex:"+domain.Value]; !ok {
					builder.AddRegexp(domain.Value)
					setMap["regex:"+domain.Value] = true
				}
			case routercommon.Domain_RootDomain:
				v := domain.Value
				if !strings.HasPrefix(v, ".") {
					v = "." + v
				}
				if _, ok := setMap["suffix:"+v]; !ok {
					builder.AddSuffix(v)
					setMap["suffix:"+v] = true
				}
			}
		}
		if builder.Len() > 0 {
			m[code] = builder
		}
		for attribute, attributeEntries := range attributes {
			attributeBuilder := domain.NewDomainSetBuilder()
			setMap := make(map[string]bool, len(attributeEntries))
			for _, domain := range attributeEntries {
				switch domain.Type {
				case routercommon.Domain_Full:
					if _, ok := setMap["full:"+domain.Value]; !ok {
						attributeBuilder.AddFull(domain.Value)
						setMap["full:"+domain.Value] = true
					}
				case routercommon.Domain_Plain:
					if _, ok := setMap["keyword:"+domain.Value]; !ok {
						attributeBuilder.AddKeyword(domain.Value)
						setMap["keyword:"+domain.Value] = true
					}
				case routercommon.Domain_Regex:
					if _, ok := setMap["regex:"+domain.Value]; !ok {
						attributeBuilder.AddRegexp(domain.Value)
						setMap["regex:"+domain.Value] = true
					}
				case routercommon.Domain_RootDomain:
					v := domain.Value
					if !strings.HasPrefix(v, ".") {
						v = "." + v
					}
					if _, ok := setMap["suffix:"+v]; !ok {
						attributeBuilder.AddSuffix(v)
						setMap["suffix:"+v] = true
					}
				}
			}
			m[attribute] = attributeBuilder
		}
	}

	s := make(map[string]*domain.DomainSet, len(m))
	for k, v := range m {
		vv, err := v.Build()
		if err != nil {
			return nil, fmt.Errorf("build domain set failed: code: %s, error: %w", k, err)
		}
		s[k] = vv
	}

	return s, nil
}

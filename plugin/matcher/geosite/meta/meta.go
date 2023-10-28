package meta

import (
	"fmt"
	"os"
	"strings"

	"github.com/rnetx/cdns/utils/domain"

	"gopkg.in/yaml.v3"
)

type metaRule struct {
	Payload []string `yaml:"payload"`
}

func ReadFile(path string) (*domain.DomainSet, int, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	var rules metaRule
	err = decoder.Decode(&rules)
	if err != nil {
		return nil, 0, err
	}
	if len(rules.Payload) == 0 {
		return nil, 0, fmt.Errorf("missing payload")
	}
	builder := domain.NewDomainSetBuilder()
	for _, rule := range rules.Payload {
		rr := strings.SplitN(rule, ",", 2)
		if len(rr) == 1 {
			switch {
			case strings.HasPrefix(rule, "+"):
				builder.AddSuffix(rule[1:])
			case strings.HasPrefix(rule, "."):
				builder.AddSuffix(rule)
			default:
				builder.AddFull(rule)
			}
		} else {
			switch rr[0] {
			case "DOMAIN":
				builder.AddFull(rr[1])
			case "DOMAIN-SUFFIX":
				switch {
				case strings.HasPrefix(rr[1], "+"):
					builder.AddSuffix(rr[1][1:])
				case strings.HasPrefix(rr[1], "*"):
					builder.AddSuffix(rr[1][1:])
				case strings.Contains(rr[1], "*"):
					return nil, 0, fmt.Errorf("invalid rule: %s", rule)
				default:
					builder.AddSuffix(rr[1])
				}
			case "DOMAIN-KEYWORD":
				builder.AddKeyword(rr[1])
			default:
				return nil, 0, fmt.Errorf("unknown rule type: %s", rr[0])
			}
		}
	}
	ss, err := builder.Build()
	if err != nil {
		return nil, 0, err
	}
	return ss, builder.Len(), nil
}

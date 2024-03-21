package restapi

import (
	"regexp"
	"strings"

	"github.com/iotaledger/hive.go/ierrors"
)

func CompileRouteAsRegex(route string) *regexp.Regexp {
	r := route

	// interpret the string as raw regex if it starts with "^"
	if !strings.HasPrefix(route, "^") {
		r = regexp.QuoteMeta(route)
		r = strings.ReplaceAll(r, `\*`, "(.*?)")
		r += "$"
	}

	reg, err := regexp.Compile(r)
	if err != nil {
		return nil
	}

	return reg
}

func CompileRoutesAsRegexes(routes []string) ([]*regexp.Regexp, error) {
	regexes := make([]*regexp.Regexp, len(routes))
	for i, route := range routes {
		reg := CompileRouteAsRegex(route)
		if reg == nil {
			return nil, ierrors.Errorf("invalid route in config: %s", route)
		}
		regexes[i] = reg
	}

	return regexes, nil
}

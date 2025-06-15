package query

import (
	crawler "github.com/amankumarsingh77/search_engine/internal/common"
	"regexp"
	"strings"
)

func Parse(rawQuery string, page, pageSize int) *QueryPlan {
	plan := &QueryPlan{
		rawQuery: rawQuery,
		page:     page,
		pageSize: pageSize,
		operator: "AND",
		filters:  make(map[string]string),
	}
	filterRegex := regexp.MustCompile(`(\w+):("([^"]+)"|(\S+))`)
	matches := filterRegex.FindAllStringSubmatch(rawQuery, -1)
	for _, match := range matches {
		filterType := strings.ToLower(match[1])
		filterValue := match[3]
		if filterValue == "" {
			filterValue = match[4]
		}
		plan.filters[filterType] = filterValue
		rawQuery = strings.Replace(rawQuery, match[0], "", 1)
	}
	if strings.Contains(rawQuery, `"`) {
		plan.operator = "PHRASE"
	} else if strings.Contains(rawQuery, "OR") {
		plan.operator = "OR"
	}
	terms := crawler.NormalizeText(rawQuery)
	for _, term := range terms {
		if term != "" {
			plan.terms = append(plan.terms, term)
		}
	}
	return plan
}

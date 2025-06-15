package query

type QueryPlan struct {
	rawQuery string
	terms    []string
	termIDs  []int64
	operator string
	page     int
	pageSize int
	filters  map[string]string
}

type SearchResult struct {
	DocID       int64
	URL         string
	Title       string
	Description string
	Score       float64
	Snippet     string
}

type ScoredDOC struct {
	DocID int64
	Score float64
}

type Posting struct {
	DocID     int64
	Positions []int32
}

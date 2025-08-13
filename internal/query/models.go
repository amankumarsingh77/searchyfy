package query

import "sync"

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
	DocID       int64   `json:"doc_id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Snippet     string  `json:"snippet"`
}

type ScoredDoc struct {
	DocID int64
	Score float64
}

type Posting struct {
	DocID     int64
	Positions []int32
}

type TermBatch struct {
	Terms   []string
	Results chan TermResult
}

type TermResult struct {
	Term  string
	ID    int64
	Error error
}

type PostingBatch struct {
	TermIDs []int64
	Results chan PostingResult
}

type PostingResult struct {
	TermID   int64
	Postings []Posting
	Error    error
}

var (
	docIDPool = sync.Pool{
		New: func() interface{} {
			return make([]int64, 0, 1000)
		},
	}

	postingPool = sync.Pool{
		New: func() interface{} {
			return make([]Posting, 0, 100)
		},
	}
)

func GetDocIDSlice() []int64 {
	return docIDPool.Get().([]int64)[:0]
}

func PutDocIDSlice(s []int64) {
	if cap(s) < 10000 {
		docIDPool.Put(s)
	}
}

func GetPostingSlice() []Posting {
	return postingPool.Get().([]Posting)[:0]
}

func PutPostingSlice(s []Posting) {
	if cap(s) < 1000 {
		postingPool.Put(s)
	}
}

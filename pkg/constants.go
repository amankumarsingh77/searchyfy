package pkg

type RawTFData map[string]map[string]int
type DocLengths map[int]int
type TFIDFScore struct {
	TokenID    int
	DocID      string
	TFIDFScore float64
}

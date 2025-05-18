package pkg

import (
	"log"
	"math"
	"sync"
)

var (
	globalRawTFData  = make(RawTFData)
	globalDocLengths = make(DocLengths)
	globalDataMutex  = &sync.Mutex{} // To protect shared globalRawTFData and globalDocLengths
)

func CalculateIDF(tokenIDToDocFreq map[int]int, totalNumberOfDocuments int) map[int]float64 {
	idfScores := make(map[int]float64)
	if totalNumberOfDocuments == 0 {
		return idfScores
	}
	for tokenID, docFreq := range tokenIDToDocFreq {
		if docFreq > 0 {
			idfScores[tokenID] = math.Log(float64(totalNumberOfDocuments) / float64(docFreq))
		} else {
			idfScores[tokenID] = 0
		}
	}
	return idfScores
}

func CalculateTFIDFValues(
	rawTFs RawTFData,
	docLengths DocLengths,
	tokenTextToIDMap map[string]int,
	idfScores map[int]float64,
) []TFIDFScore {
	log.Println("Logic: Calculating TF-IDF scores...")
	var tfidfRecords []TFIDFScore

	for tokenText, docMap := range rawTFs {
		tokenID, ok := tokenTextToIDMap[tokenText]
		if !ok {
			log.Printf("Logic Warning: Token text '%s' not found in tokenTextToIDMap during TF-IDF calculation. Skipping.", tokenText)
			continue
		}

		idf := idfScores[tokenID]
		// if idf == 0 { // Terms with IDF 0 (e.g., in all docs with some formulas) might be skipped
		//  log.Printf("Logic Info: Token ID %d ('%s') has IDF of 0. Skipping TF-IDF calculation for its occurrences.", tokenID, tokenText)
		//  continue
		// }

		for docID, rawTFCount := range docMap {
			docLen, docExists := docLengths[docID]
			if !docExists {
				log.Printf("Logic Warning: Document ID '%s' not found in docLengths. Skipping TF-IDF for token '%s' in this document.", docID, tokenText)
				continue
			}

			var normalizedTF float64
			if docLen > 0 {
				normalizedTF = float64(rawTFCount) / float64(docLen)
			} else {
				// log.Printf("Logic Warning: Document ID '%s' has length 0. Setting TF to 0 for token '%s'.", docID, tokenText)
				normalizedTF = 0 // Avoid division by zero; TF is 0 if doc length is 0.
			}

			tfidfScoreValue := normalizedTF * idf
			if tfidfScoreValue > 0 { // Only create record if TF-IDF is meaningful
				tfidfRecords = append(tfidfRecords, TFIDFScore{
					TokenID:    tokenID,
					DocID:      docID,
					TFIDFScore: tfidfScoreValue,
				})
			}
		}
	}
	log.Printf("Logic: Calculated %d TF-IDF scores with values > 0.", len(tfidfRecords))
	return tfidfRecords
}

func AddToIndexAndDocLength(rawTFs RawTFData, docLengths DocLengths, tokens []string, docID int64) {
	globalDataMutex.Lock() // Protect shared maps
	defer globalDataMutex.Unlock()

	if _, exists := docLengths[docID]; !exists {
		docLengths[docID] = 0
	}
	docLengths[docID] += len(tokens)

	for _, token := range tokens {
		if _, exists := rawTFs[token]; !exists {
			rawTFs[token] = make(map[int64]int)
		}
		rawTFs[token][docID]++
	}
}

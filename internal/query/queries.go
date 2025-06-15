package query

const (
	getTopNQuery = `SELECT term_id 
						FROM (
							SELECT term_id, COUNT(*) AS freq
							FROM postings
							GROUP BY term_id
							ORDER BY freq DESC
							LIMIT $1
						) AS top_terms  ;
					`
	getAvgTokenCount      = `SELECT AVG(token_count) FROM documents`
	getTotalNoDocs        = `SELECT COUNT(*) FROM documents`
	getDocumentTokenCount = `SELECT token_count FROM documents WHERE id = $1`
	getDocFrequency       = `
							SELECT COUNT(DISTINCT doc_id) 
							FROM postings 
							WHERE term_id = $1
						`
	getMultipleDocs = `
						SELECT id, url, title, description 
						FROM documents 
						WHERE id = ANY($1)
						ORDER BY array_position($2, id)  -- Preserve original order
					`
	getTerms            = `SELECT term, id FROM terms WHERE term = ANY($1)`
	getPostingsByTermID = `SELECT doc_id, positions FROM postings WHERE term_id = $1`
	getDocIDsByTermID   = `
							SELECT doc_id
							FROM postings
							WHERE term_id = ANY($1)
							GROUP BY doc_id
						`
	getDocByFilters = `
						SELECT p.doc_id
						FROM postings p
						JOIN documents d ON d.id = p.doc_id
						WHERE p.term_id = ANY($1)
						AND d.url LIKE '%' || $2 || '%'
						GROUP BY p.doc_id
					`
	getDocByPhrase = `
						SELECT p1.doc_id
						FROM postings p1
						JOIN postings p2 ON p1.doc_id = p2.doc_id
						WHERE p1.term_id = $1
						AND p2.term_id = $2
						AND EXISTS (
							SELECT 1
							FROM unnest(p1.positions) pos1
							JOIN unnest(p2.positions) pos2 ON pos2 = pos1 + 1
						)
					`
	getDescriptionLength = `
		SELECT char_length(description) FROM documents WHERE id = $1
	`
	getPostingOfTermLength = `
							SELECT array_length(positions, 1) 
							FROM postings 
							WHERE doc_id = $1 AND term_id = $2
						`
)

package query

const (
	getTopNQuery = `
		SELECT term_id 
		FROM postings 
		GROUP BY term_id 
		ORDER BY SUM(frequency) DESC 
		LIMIT $1
	`

	getAvgTokenCount = `SELECT AVG(token_count)::float FROM documents`

	getTotalNoDocs = `SELECT COUNT(*)::int FROM documents`

	getTermsBatch = `
		SELECT term, id FROM terms 
		WHERE term = ANY($1)
	`

	getPostingsByTermIDBatch = `
		SELECT term_id, doc_id, positions 
		FROM postings 
		WHERE term_id = ANY($1)
		ORDER BY term_id, doc_id
	`

	getDocFrequencyBatch = `
		SELECT term_id, COUNT(DISTINCT doc_id) as doc_frequency
		FROM postings
		WHERE term_id = ANY($1)
		GROUP BY term_id
	`

	getBooleanIntersection = `
		WITH term_docs AS (
			SELECT term_id, doc_id
			FROM postings
			WHERE term_id = ANY($1)
		),
		doc_term_counts AS (
			SELECT doc_id, COUNT(DISTINCT term_id) as term_count
			FROM term_docs
			GROUP BY doc_id
		)
		SELECT doc_id
		FROM doc_term_counts
		WHERE term_count = $2
	`

	getBooleanUnion = `
		SELECT DISTINCT doc_id
		FROM postings
		WHERE term_id = ANY($1)
	`

	getPhraseSearch = `
		WITH RECURSIVE phrase_matches AS (
			SELECT 
				p1.doc_id,
				p1.positions[1] as start_pos,
				1 as term_index,
				ARRAY[p1.positions[1]] as matched_positions
			FROM postings p1
			WHERE p1.term_id = $1
			
			UNION ALL
			
			SELECT 
				pm.doc_id,
				pm.start_pos,
				pm.term_index + 1,
				pm.matched_positions || (pm.start_pos + pm.term_index)
			FROM phrase_matches pm
			JOIN postings p ON p.doc_id = pm.doc_id 
				AND p.term_id = $2
				AND (pm.start_pos + pm.term_index) = ANY(p.positions)
			WHERE pm.term_index < $3
		)
		SELECT DISTINCT doc_id
		FROM phrase_matches
		WHERE term_index = $3
	`

	getSiteFilteredDocs = `
		SELECT id FROM documents 
		WHERE url LIKE '%' || $1 || '%'
	`

	createTermFrequencyView = `
		CREATE MATERIALIZED VIEW IF NOT EXISTS term_frequencies AS
		SELECT 
			term_id,
			COUNT(DISTINCT doc_id) as doc_frequency,
			SUM(frequency) as total_frequency
		FROM postings
		GROUP BY term_id
		WITH DATA;
		
		CREATE UNIQUE INDEX IF NOT EXISTS idx_term_frequencies_term_id 
		ON term_frequencies(term_id);
		
		CREATE INDEX IF NOT EXISTS idx_term_frequencies_frequency 
		ON term_frequencies(total_frequency DESC);
	`

	refreshTermFrequencies = `REFRESH MATERIALIZED VIEW CONCURRENTLY term_frequencies`

	createOptimizedIndexes = `
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_postings_term_doc 
		ON postings(term_id, doc_id);
		
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_postings_doc_term 
		ON postings(doc_id, term_id);
		
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_postings_positions_gin 
		ON postings USING GIN(positions);
		
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_documents_url 
		ON documents(url);
		
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_documents_token_count 
		ON documents(token_count);
		
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_terms_term 
		ON terms(term);
	`

	getDocumentsBatch = `
		SELECT id, url, title, description, token_count 
		FROM documents 
		WHERE id = ANY($1)
		ORDER BY CASE 
			WHEN id = ANY($2) THEN array_position($2, id)
			ELSE 999999
		END
	`
)

package indexer

const (
	insertDocuments = `INSERT INTO documents (url, title, description, token_count)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT(url) DO UPDATE SET 
								title = EXCLUDED.title,
								description = EXCLUDED.description,
						    	token_count= EXCLUDED.token_count,
								indexed_at=NOW()
						RETURNING id
						`
	insertMissingTerms = `INSERT INTO terms (term)
							SELECT unnest($1::text[])
							ON CONFLICT (term) DO NOTHING
							`
	getIDsByTerms  = `SELECT id, term FROM terms WHERE term = ANY($1::text[])`
	insertPostings = `INSERT INTO postings (term_id, doc_id, positions)
				VALUES ($1, $2, $3)
				ON CONFLICT (term_id, doc_id) DO UPDATE SET
					positions = EXCLUDED.positions`
)

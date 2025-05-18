package crawler

const (
	getDocIDByURL      = `SELECT doc_id FROM webpages WHERE url=$1`
	updateWebPage      = `UPDATE webpage SET title=$1, description=$2, keywords=$3, updated_at=CURRENT_TIMESTAMP WHERE doc_id=$4`
	addOrUpdateWebpage = `INSERT INTO webpages (url, title, description, keywords)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (url) DO UPDATE SET
            title = EXCLUDED.title,
            description = EXCLUDED.description,
            keywords = EXCLUDED.keywords,
            updated_at = CURRENT_TIMESTAMP
        RETURNING doc_id`
	getToken       = `SELECT id FROM tokens WHERE token_text = $1`
	insertToken    = `INSERT INTO tokens (token_text) VALUES ($1) RETURNING id`
	insertTokenDOC = `
        INSERT INTO token_documents (token_id, doc_id, tfidf_score)
        VALUES ($1, $2, $3)
        ON CONFLICT (token_id, doc_id) DO UPDATE SET tfidf_score = EXCLUDED.tfidf_score
    `
)

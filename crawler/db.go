package crawler

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/models"
	"github.com/amankumarsingh77/search_engine/pkg"
	"github.com/jmoiron/sqlx"
	"log"
	"strings"
)

type DB struct {
	conn *sqlx.DB
}

func NewDBConnection(cfg *config.PostgresConfig) (*DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName)
	if cfg.SSL {
		dsn += "sslmode=require"
	} else {
		dsn += "sslmode=disable"
	}
	conn, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("db connection error :%v", err)
	}
	return &DB{
		conn: conn,
	}, nil
}

func (d *DB) AddWebpage(page models.WebPage) (int64, error) {
	keywords := strings.Join(page.Keywords, ",")
	var docID int64
	err := d.conn.Get(&docID, getDocIDByURL, page.URL)
	if err == nil {
		_, err = d.conn.Exec(updateWebPage, page.Title, page.Description, page.Keywords, docID)
		if err != nil {
			return 0, fmt.Errorf("failed to update webpage (doc_id: %d): %w", docID, err)
		}
		return docID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to check existing webpage by URL %s: %w", page.URL, err)
	}

	err = d.conn.QueryRowx(addOrUpdateWebpage, page.URL, page.Title, page.Description, keywords).Scan(&docID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert/update webpage %s: %w", page.URL, err)
	}
	return docID, nil
}

func (d *DB) GetOrInsertTokenID(tx *sqlx.Tx, tokenText string) (int64, error) {
	var tokenID int64
	err := tx.Get(&tokenID, getToken, tokenText)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowx(insertToken, tokenText).Scan(&tokenID)
		if err != nil {
			return 0, fmt.Errorf("failed to insert token '%s': %w", tokenText, err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("failed to query token '%s': %w", tokenText, err)
	}
	return tokenID, nil
}

func (d *DB) ProcessTokensInDB(db *sqlx.DB, rawTFs pkg.RawTFData) (map[string]int64, map[int64]int, error) {
	log.Println("DB: Populating tokens table and collecting document frequencies...")
	tokenTextToIDMap := make(map[string]int64)
	tokenIDToDocFreq := make(map[int64]int) // Stores document frequency for each token_id

	tx, err := db.Beginx()
	if err != nil {
		return nil, nil, fmt.Errorf("DB: failed to begin transaction for token processing: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	for tokenText, docMap := range rawTFs {
		tokenID, err := d.GetOrInsertTokenID(tx, tokenText)
		if err != nil {
			return nil, nil, fmt.Errorf("DB: error processing token '%s': %w", tokenText, err)
		}
		tokenTextToIDMap[tokenText] = tokenID
		tokenIDToDocFreq[tokenID] = len(docMap)
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("DB: failed to commit transaction for token processing: %w", err)
	}
	log.Printf("DB: Processed %d unique tokens.", len(tokenTextToIDMap))
	return tokenTextToIDMap, tokenIDToDocFreq, nil
}

// storeTFIDFRecordsInDB inserts or updates TF-IDF scores in the database.
// It handles its own transaction for batch processing.
func StoreTFIDFRecordsInDB(db *sqlx.DB, tfidfRecords []pkg.TFIDFScore) error {
	if len(tfidfRecords) == 0 {
		log.Println("DB: No TF-IDF records to store.")
		return nil
	}
	log.Printf("DB: Storing %d TF-IDF records...", len(tfidfRecords))

	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("DB: failed to begin transaction for TF-IDF storage: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Preparex(insertToken)
	if err != nil {
		return fmt.Errorf("DB: failed to prepare TF-IDF insert statement: %w", err)
	}
	defer stmt.Close()

	for i, record := range tfidfRecords {
		_, err := stmt.Exec(record.TokenID, record.DocID, record.TFIDFScore)
		if err != nil {
			return fmt.Errorf("DB: failed to insert/update TF-IDF for token_id %d, doc_id %s: %w", record.TokenID, record.DocID, err)
		}
		if (i+1)%1000 == 0 {
			log.Printf("DB: Inserted/Updated %d TF-IDF scores...", i+1)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("DB: failed to commit TF-IDF scores: %w", err)
	}
	log.Printf("DB: Successfully stored %d TF-IDF records.", len(tfidfRecords))
	return nil
}

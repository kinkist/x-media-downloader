package db

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"

	"github.com/kinkist/x-media-downloader/logger"
)

// DB is the global MySQL connection instance.
// nil means DB tracking is disabled.
var DB *sql.DB

// Config holds MySQL connection information.
type Config struct {
	Host   string
	User   string
	Pass   string
	DBName string
}

// Init connects to MySQL and initializes the download tracking table.
// Creates the database automatically if it does not exist.
// On failure at any step, DB is set back to nil and the error is returned.
// Callers may ignore the error and continue with DB==nil (tracking disabled).
func Init(cfg Config) error {
	// ── Step 1: connect to server without DB name to check/create the database ──
	dsnNoDB := fmt.Sprintf("%s:%s@tcp(%s)/?parseTime=true&charset=utf8mb4",
		cfg.User, cfg.Pass, cfg.Host)
	logger.Debug("connecting to DB server (no database specified): %s:***@tcp(%s)/", cfg.User, cfg.Host)

	tmpDB, err := sql.Open("mysql", dsnNoDB)
	if err != nil {
		return fmt.Errorf("failed to open DB (server connection): %w", err)
	}
	if err = tmpDB.Ping(); err != nil {
		tmpDB.Close()
		return fmt.Errorf("DB ping failed (server connection): %w", err)
	}
	logger.Debug("DB server ping OK, checking/creating database: %s", cfg.DBName)

	_, err = tmpDB.Exec(fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		cfg.DBName,
	))
	tmpDB.Close()
	if err != nil {
		return fmt.Errorf("failed to create database (%s): %w", cfg.DBName, err)
	}
	logger.Debug("database checked/created: %s", cfg.DBName)

	// ── Step 2: reconnect with the DB name included in the DSN ──
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4",
		cfg.User, cfg.Pass, cfg.Host, cfg.DBName)
	logger.Debug("connecting to DB: %s:***@tcp(%s)/%s", cfg.User, cfg.Host, cfg.DBName)

	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		DB = nil
		return fmt.Errorf("failed to open DB: %w", err)
	}
	logger.Debug("sql.Open succeeded, attempting ping...")

	if err = DB.Ping(); err != nil {
		DB.Close()
		DB = nil
		return fmt.Errorf("DB ping failed: %w", err)
	}
	logger.Debug("DB ping OK")

	if err = ensureTable(); err != nil {
		DB.Close()
		DB = nil
		return fmt.Errorf("failed to initialize table: %w", err)
	}
	logger.Debug("DB initialized (host=%s, db=%s)", cfg.Host, cfg.DBName)
	return nil
}

// ensureTable creates the downloaded_files table if it does not exist
// and applies schema migrations for older schemas.
func ensureTable() error {
	logger.Debug("checking/creating downloaded_files table...")
	_, err := DB.Exec(`CREATE TABLE IF NOT EXISTS downloaded_files (
		id         BIGINT       AUTO_INCREMENT PRIMARY KEY,
		http_url   VARCHAR(512) NOT NULL DEFAULT '',
		file_path  VARCHAR(512) NOT NULL DEFAULT '',
		tweet_id   VARCHAR(64)   NOT NULL,
		username   VARCHAR(128)  NOT NULL,
		user_id    VARCHAR(64)   NOT NULL,
		file_type  VARCHAR(16)   NOT NULL COMMENT 'image|video|text',
		is_retweet TINYINT(1)    NOT NULL DEFAULT 0,
		created_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uk_http_url (http_url)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	migrateSchema()
	logger.Debug("table ready")
	return nil
}

// migrateSchema upgrades an old schema (missing http_url column) to the current schema.
// Each ALTER failure is treated as already applied or not applicable and is ignored.
func migrateSchema() {
	// add http_url column (only if missing)
	if _, err := DB.Exec(`ALTER TABLE downloaded_files ADD COLUMN http_url VARCHAR(512) NOT NULL DEFAULT '' AFTER id`); err != nil {
		logger.Debug("skip adding http_url column (already exists or error): %v", err)
	}
	// add new unique key (only if missing)
	if _, err := DB.Exec(`ALTER TABLE downloaded_files ADD UNIQUE KEY uk_http_url (http_url)`); err != nil {
		logger.Debug("skip adding uk_http_url key (already exists or error): %v", err)
	}
	// remove old unique key (only if present)
	if _, err := DB.Exec(`ALTER TABLE downloaded_files DROP INDEX uk_file_path`); err != nil {
		logger.Debug("skip dropping uk_file_path key (not found or error): %v", err)
	}
}

// IsURLTracked reports whether httpURL has been recorded in the DB.
// Returns true if the URL was already downloaded and should be skipped.
func IsURLTracked(httpURL string) (bool, error) {
	logger.Debug("DB query (URL): %s", httpURL)
	var count int
	if scanErr := DB.QueryRow(
		"SELECT COUNT(*) FROM downloaded_files WHERE http_url = ?", httpURL,
	).Scan(&count); scanErr != nil {
		return false, fmt.Errorf("DB query failed: %w", scanErr)
	}
	tracked := count > 0
	logger.Debug("DB query result: tracked=%v", tracked)
	return tracked, nil
}

// MarkFileDownloaded records a successfully downloaded file in the DB.
func MarkFileDownloaded(httpURL, filePath, tweetID, username, userID, fileType string, isRetweet bool) error {
	logger.Debug("DB record: url=%s file=%s tweet=%s user=%s type=%s rt=%v",
		httpURL, filePath, tweetID, username, fileType, isRetweet)
	rtVal := 0
	if isRetweet {
		rtVal = 1
	}
	_, err := DB.Exec(
		`INSERT IGNORE INTO downloaded_files
		 (http_url, file_path, tweet_id, username, user_id, file_type, is_retweet)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		httpURL, filePath, tweetID, username, userID, fileType, rtVal,
	)
	if err != nil {
		return fmt.Errorf("DB record failed: %w", err)
	}
	logger.Debug("DB record complete")
	return nil
}

package importer

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register modernc/sqlite driver

	"github.com/quipthread/quipthread/models"
)

// ColumnInfo describes a single column in an inspected table.
type ColumnInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Samples []string `json:"samples"`
}

// TableInfo describes an inspected table.
type TableInfo struct {
	Name    string       `json:"name"`
	Columns []ColumnInfo `json:"columns"`
}

// ColumnMapping describes how to map an arbitrary SQLite table to Quipthread
// comment fields. All values are column names from the source table.
// At least one of PageID / PageURL and Content must be set.
type ColumnMapping struct {
	Table       string            `json:"table"`
	Columns     map[string]string `json:"columns"`      // quipthread_field → source_column
	StripDomain bool              `json:"strip_domain"` // derive page_id from page_url
	WrapInP     bool              `json:"wrap_in_p"`    // wrap plain-text content in <p>
}

// openReadOnly opens a SQLite file read-only. The caller is responsible for
// closing the returned *sql.DB.
func openReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_journal=off", path))
}

// InspectSQLite returns the schema of every table in the file at path,
// including up to 3 sample values per column.
func InspectSQLite(path string) ([]TableInfo, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	tableRows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer tableRows.Close()

	var tableNames []string
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err == nil {
			tableNames = append(tableNames, name)
		}
	}

	var tables []TableInfo
	for _, tbl := range tableNames {
		cols, err := inspectTable(db, tbl)
		if err != nil {
			continue
		}
		tables = append(tables, TableInfo{Name: tbl, Columns: cols})
	}
	return tables, nil
}

func inspectTable(db *sql.DB, table string) ([]ColumnInfo, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			continue
		}
		cols = append(cols, ColumnInfo{Name: name, Type: colType})
	}

	// Collect up to 3 sample values per column.
	for i, col := range cols {
		sampleRows, err := db.Query(
			fmt.Sprintf(`SELECT %q FROM %q WHERE %q IS NOT NULL LIMIT 3`, col.Name, table, col.Name))
		if err != nil {
			continue
		}
		for sampleRows.Next() {
			var v string
			if err := sampleRows.Scan(&v); err == nil {
				if len(v) > 80 {
					v = v[:77] + "..."
				}
				cols[i].Samples = append(cols[i].Samples, v)
			}
		}
		sampleRows.Close()
	}
	return cols, nil
}

// ImportQuipthreadDB reads all users and comments from an uploaded Quipthread
// SQLite database. Callers override siteID when inserting into the target DB.
func ImportQuipthreadDB(path string) (*Result, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	users, err := readQuipthreadUsers(db)
	if err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}
	comments, err := readQuipthreadComments(db)
	if err != nil {
		return nil, fmt.Errorf("read comments: %w", err)
	}
	return &Result{Users: users, Comments: comments}, nil
}

func readQuipthreadUsers(db *sql.DB) ([]*models.User, error) {
	rows, err := db.Query(
		`SELECT id, display_name, COALESCE(email,''), COALESCE(avatar_url,''), role, banned, created_at FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*models.User
	for rows.Next() {
		var u models.User
		var banned int
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Email, &u.AvatarURL, &u.Role, &banned, &u.CreatedAt); err == nil {
			u.Banned = banned != 0
			users = append(users, &u)
		}
	}
	return users, nil
}

func readQuipthreadComments(db *sql.DB) ([]*models.Comment, error) {
	rows, err := db.Query(`
		SELECT id, site_id, page_id,
		       COALESCE(page_url,''), COALESCE(page_title,''),
		       COALESCE(parent_id,''), user_id, content, status,
		       COALESCE(imported,0), COALESCE(disqus_author,''),
		       created_at, updated_at
		FROM comments`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []*models.Comment
	for rows.Next() {
		var c models.Comment
		var imported int
		if err := rows.Scan(
			&c.ID, &c.SiteID, &c.PageID, &c.PageURL, &c.PageTitle,
			&c.ParentID, &c.UserID, &c.Content, &c.Status,
			&imported, &c.DisqusAuthor, &c.CreatedAt, &c.UpdatedAt,
		); err == nil {
			c.Imported = imported != 0
			comments = append(comments, &c)
		}
	}
	return comments, nil
}

// ImportMappedSQLite reads rows from the specified table using the provided
// column mapping and converts them to Quipthread comments.
func ImportMappedSQLite(path string, m ColumnMapping, allowedCols map[string]bool) (*Result, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	// Build SELECT list from mapping, validating every column name.
	selectCols := make(map[string]string) // alias → col name
	for field, col := range m.Columns {
		if col == "" {
			continue
		}
		if !allowedCols[col] {
			return nil, fmt.Errorf("unknown column %q", col)
		}
		selectCols[field] = col
	}

	var selects []string
	for field, col := range selectCols {
		selects = append(selects, fmt.Sprintf("%q AS %q", col, field))
	}
	if len(selects) == 0 {
		return nil, fmt.Errorf("no column mappings provided")
	}

	query := fmt.Sprintf(`SELECT %s FROM %q`, strings.Join(selects, ", "), m.Table) //nolint:gosec // table name from validated config mapping, not user input
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	users := make(map[string]*models.User)
	var comments []*models.Comment

	for rows.Next() {
		vals := make([]interface{}, len(colNames))
		ptrs := make([]interface{}, len(colNames))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}

		row := make(map[string]string, len(colNames))
		for i, name := range colNames {
			if vals[i] != nil {
				row[name] = fmt.Sprintf("%v", vals[i])
			}
		}

		content := row["content"]
		if m.WrapInP && content != "" && !strings.Contains(content, "<") {
			content = "<p>" + strings.ReplaceAll(strings.TrimSpace(content), "\n\n", "</p><p>") + "</p>"
		}

		pageURL := row["page_url"]
		pageID := row["page_id"]
		if pageID == "" || m.StripDomain {
			pageID = pageIDFromURL(pageURL)
		}

		authorName := row["author_name"]
		authorKey := authorName
		if authorKey == "" {
			authorKey = "anonymous"
		}
		userID := syntheticUserID("sqlite", authorKey)
		if _, ok := users[userID]; !ok {
			users[userID] = &models.User{
				ID:          userID,
				DisplayName: authorName,
				AvatarURL:   row["author_avatar"],
				Role:        "commenter",
			}
		}

		status := row["status"]
		if status == "" {
			status = "approved"
		}

		var createdAt time.Time
		if ts := row["created_at"]; ts != "" {
			for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"} {
				if t, err := time.Parse(layout, ts); err == nil {
					createdAt = t
					break
				}
			}
		}

		comments = append(comments, &models.Comment{
			ID:           row["id"],
			PageID:       pageID,
			PageURL:      pageURL,
			PageTitle:    row["page_title"],
			ParentID:     row["parent_id"],
			UserID:       userID,
			Content:      content,
			Status:       status,
			Imported:     true,
			DisqusAuthor: authorName,
			CreatedAt:    createdAt,
		})
	}

	result := &Result{Comments: comments}
	for _, u := range users {
		result.Users = append(result.Users, u)
	}
	return result, nil
}

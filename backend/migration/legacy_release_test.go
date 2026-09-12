package migration

import "testing"

func TestReleasedDatabaseBaseline(t *testing.T) {
	conn, target := legacyFixtureDatabase(t, "released_v0_1_1_schema.sql")
	if _, err := conn.ExecContext(t.Context(), `INSERT INTO users(id,display_name) VALUES ('old-user','Existing'); INSERT INTO sites(id,owner_id,domain) VALUES ('old-site','old-user','example.invalid'); INSERT INTO comments(id,site_id,page_id,user_id,content) VALUES ('old-comment','old-site','/','old-user','Preserve me')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		version, err := legacyBaseline(t.Context(), target)
		if err != nil || version != "00001" {
			t.Fatalf("baseline=%q error=%v", version, err)
		}
	}
	var content string
	var defaults, tables int
	if err := conn.QueryRowContext(t.Context(), `SELECT content,upvotes+flags FROM comments WHERE id='old-comment'`).Scan(&content, &defaults); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_schema WHERE name IN ('comment_flags','comment_votes')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if content != "Preserve me" || defaults != 0 || tables != 2 {
		t.Fatal("released data or baseline changed")
	}
}

func TestReleasedDatabaseRejectsPartialTableBeforeWriting(t *testing.T) {
	conn, target := legacyFixtureDatabase(t, "released_v0_1_1_schema.sql")
	if _, err := conn.ExecContext(t.Context(), `CREATE TABLE comment_votes(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyBaseline(t.Context(), target); err == nil {
		t.Fatal("accepted partial table")
	}
	var count int
	if err := conn.QueryRowContext(t.Context(), `SELECT count(*) FROM pragma_table_info('comments') WHERE name IN ('upvotes','flags')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("modified rejected schema")
	}
}

package migration

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
)

func atlasDatabaseURL(databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "sqlite" {
		return databaseURL, nil
	}
	path, err := filepath.Abs(u.Path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(path))
		if parentErr != nil {
			return "", parentErr
		}
		canonical, err = filepath.Join(parent, filepath.Base(path)), nil
	}
	if err != nil {
		return "", err
	}
	// A file: DSN enables Atlas SQLite's built-in per-database lock.
	return (&url.URL{Scheme: "sqlite", Host: "file:", Path: canonical}).String(), nil
}

// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	_ "modernc.org/sqlite"
)

type sqliteRepository struct {
	db     *sql.DB
	lock   *os.File
	people map[string]person
}

func openSQLite(path string, people map[string]person) (*sqliteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, errors.New("another process owns this catalog; one replica required")
	}
	cleanup := func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }
	// Precreation prevents a world-readable database before SQLite opens it.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		cleanup()
		return nil, err
	}
	file.Close()
	if err = os.Chmod(path, 0600); err != nil {
		cleanup()
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		cleanup()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; CREATE TABLE IF NOT EXISTS catalogs (user_id TEXT PRIMARY KEY, version INTEGER NOT NULL, body BLOB NOT NULL); CREATE TABLE IF NOT EXISTS configuration (id INTEGER PRIMARY KEY CHECK(id=1), body BLOB NOT NULL);`)
	if err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	return &sqliteRepository{db: db, lock: f, people: people}, nil
}
func (r *sqliteRepository) Close() error {
	err := r.db.Close()
	syscall.Flock(int(r.lock.Fd()), syscall.LOCK_UN)
	r.lock.Close()
	return err
}
func (r *sqliteRepository) Load(ctx context.Context, id string) (catalog, int, error) {
	if _, ok := r.people[id]; !ok {
		return catalog{}, 0, errors.New("unknown user")
	}
	var body []byte
	var n int
	var c catalog
	err := r.db.QueryRowContext(ctx, "SELECT version,body FROM catalogs WHERE user_id=?", id).Scan(&n, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return c, 0, nil
	}
	if err != nil {
		return c, 0, err
	}
	err = json.Unmarshal(body, &c)
	return c, n, err
}
func (r *sqliteRepository) Save(ctx context.Context, id string, n int, c catalog) error {
	p, ok := r.people[id]
	if !ok || c.Person != id || c.Issuer != p.Issuer || c.Subject != p.Subject {
		return errors.New("catalog identity mismatch")
	}
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var result sql.Result
	if n == 0 {
		result, err = tx.ExecContext(ctx, "INSERT INTO catalogs(user_id,version,body) VALUES(?,1,?) ON CONFLICT(user_id) DO NOTHING", id, body)
	} else {
		result, err = tx.ExecContext(ctx, "UPDATE catalogs SET version=version+1,body=? WHERE user_id=? AND version=?", body, id, n)
	}
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errConflict
	}
	return tx.Commit()
}
func (r *sqliteRepository) Credentials(_ context.Context, id string) (credentials, error) {
	p, ok := r.people[id]
	if !ok {
		return credentials{}, errors.New("unknown user")
	}
	return p.Credentials, nil
}

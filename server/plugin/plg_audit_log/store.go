package plg_audit_log

import (
	"database/sql"
	"time"
)

// Event is a single audited change. User/Session are non-reversible ids derived
// from the session (GenerateID); no password or raw credential is ever stored.
type Event struct {
	Timestamp int64
	User      string
	Action    string
	Backend   string
	Path      string
	Target    string
	Size      int64
	Session   string
	RequestID string
}

type Filter struct {
	From, To                                     int64
	Action, Path, Backend, Session, User, Target string
	Limit                                        int
}

type Store struct{ db *sql.DB }

// dedupWindow collapses repeated content writes of the same file by the same
// session (an editor can save many times) into a single entry per window.
const dedupWindow int64 = 24 * 60 * 60

func NewStore(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS audit (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		ts         INTEGER NOT NULL,
		user       TEXT NOT NULL DEFAULT '',
		action     TEXT NOT NULL,
		backend    TEXT NOT NULL DEFAULT '',
		path       TEXT NOT NULL DEFAULT '',
		target     TEXT NOT NULL DEFAULT '',
		size       INTEGER NOT NULL DEFAULT 0,
		session    TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return nil, err
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS audit_ts ON audit(ts)`)
	return &Store{db: db}, nil
}

func (s *Store) Record(ev Event) error {
	if ev.Timestamp == 0 {
		ev.Timestamp = time.Now().Unix()
	}
	if ev.Action == "save_file" {
		var one int
		err := s.db.QueryRow(
			`SELECT 1 FROM audit WHERE action='save_file' AND session=? AND path=? AND ts > ? LIMIT 1`,
			ev.Session, ev.Path, ev.Timestamp-dedupWindow,
		).Scan(&one)
		if err == nil {
			// Already recorded within the window: bump its timestamp, no new row.
			_, uerr := s.db.Exec(
				`UPDATE audit SET ts=? WHERE action='save_file' AND session=? AND path=? AND ts > ?`,
				ev.Timestamp, ev.Session, ev.Path, ev.Timestamp-dedupWindow,
			)
			return uerr
		} else if err != sql.ErrNoRows {
			return err
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO audit (ts,user,action,backend,path,target,size,session,request_id)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		ev.Timestamp, ev.User, ev.Action, ev.Backend, ev.Path, ev.Target, ev.Size, ev.Session, ev.RequestID,
	)
	return err
}

// Prune enforces retention: drop entries older than maxAgeDays, then trim the
// table to at most maxRows (keeping the most recent). Zero disables a bound.
func (s *Store) Prune(maxAgeDays, maxRows int) error {
	if maxAgeDays > 0 {
		cutoff := time.Now().Unix() - int64(maxAgeDays)*86400
		if _, err := s.db.Exec(`DELETE FROM audit WHERE ts < ?`, cutoff); err != nil {
			return err
		}
	}
	if maxRows > 0 {
		if _, err := s.db.Exec(
			`DELETE FROM audit WHERE id NOT IN (SELECT id FROM audit ORDER BY id DESC LIMIT ?)`,
			maxRows,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Query(f Filter) ([]Event, error) {
	q := `SELECT ts,user,action,backend,path,target,size,session,request_id FROM audit WHERE 1=1`
	args := []any{}
	add := func(cond string, v any) { q += cond; args = append(args, v) }
	if f.From > 0 {
		add(" AND ts >= ?", f.From)
	}
	if f.To > 0 {
		add(" AND ts <= ?", f.To)
	}
	if f.Action != "" {
		add(" AND action = ?", f.Action)
	}
	if f.Path != "" {
		add(" AND path LIKE ?", "%"+f.Path+"%")
	}
	if f.Backend != "" {
		add(" AND backend LIKE ?", "%"+f.Backend+"%")
	}
	if f.Session != "" {
		add(" AND session LIKE ?", "%"+f.Session+"%")
	}
	if f.User != "" {
		add(" AND user LIKE ?", "%"+f.User+"%")
	}
	if f.Target != "" {
		add(" AND target LIKE ?", "%"+f.Target+"%")
	}
	if f.Limit <= 0 {
		f.Limit = 200
	}
	q += " ORDER BY ts DESC LIMIT ?"
	args = append(args, f.Limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Timestamp, &e.User, &e.Action, &e.Backend, &e.Path, &e.Target, &e.Size, &e.Session, &e.RequestID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

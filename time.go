package main

import (
	"database/sql"
	"time"
)

type SQLTime struct {
	time.Time
}

func (st *SQLTime) Scan(value any) error {
	switch v := value.(type) {
	case string:
		t, err := time.Parse(time.DateTime, v)
		if err != nil {
			return err
		}
		st.Time = t
	default:
		return sql.ErrNoRows
	}
	return nil
}

func (st *SQLTime) FormatToDB() string {
	return st.Format(time.DateTime)
}

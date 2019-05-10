package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type dactfile struct {
	id   string
	path string
}

/** Returns the paths to the dbxml database files for a corpus */
func getDactFiles(db *sql.DB, corpus string) ([]dactfile, error) {
	dactfiles := make([]dactfile, 0)
	rows, errval := db.Query(fmt.Sprintf("SELECT `id`, `arch` FROM `%s_c_%s_arch` ORDER BY `id`", Cfg.Prefix, corpus))
	if errval != nil {
		return dactfiles, errval
	}

	defer rows.Close()

	for rows.Next() {
		var df dactfile
		errval = rows.Scan(&df.id, &df.path)
		if errval != nil {
			rows.Close()
			return dactfiles, errval
		}

		if strings.HasSuffix(df.path, ".dact") {
			dactfiles = append(dactfiles, df)
		}
	}
	errval = rows.Err()
	return dactfiles, errval // probably nil
}

func getDactFileById(db *sql.DB, corpus string, id string) (dactfile, error) {
	df := dactfile{id: id}

	rows, errval := db.Query(fmt.Sprintf("SELECT `arch` FROM %s_c_%s_arch WHERE id = ?", Cfg.Prefix, corpus), id)
	if errval != nil {
		return df, errval
	}

	defer rows.Close()

	if !rows.Next() {
		return df, errors.New("No dact file by id " + id + " exists.")
	}

	errval = rows.Scan(&df.path)
	if errval != nil {
		rows.Close()
		return df, errval
	}

	if !strings.HasSuffix(df.path, ".dact") {
		return df, errors.New("No dact file by id " + id + " exists.")
	}

	return df, nil
}

func mayAccess(q *Context, corpus string) bool {
	allowed, corpusExists := q.prefixes[corpus]
	return corpusExists && allowed
}

func gretelSendErr(message string, q *Context, err error) bool {
	if logerr(err) {
		http.Error(q.w, message+": "+err.Error(), 500)
		return true
	}

	return false
}

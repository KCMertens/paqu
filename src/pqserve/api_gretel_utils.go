package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

/** Returns the paths to the dbxml database files for a corpus */
func getDactFiles(db *sql.DB, corpus string) ([]string, error) {
	dactfiles := make([]string, 0)
	rows, errval := db.Query(fmt.Sprintf("SELECT `arch` FROM `%s_c_%s_arch` ORDER BY `id`", Cfg.Prefix, corpus))
	if errval != nil {
		return dactfiles, errval
	}

	for rows.Next() {
		var s string
		errval = rows.Scan(&s)
		if errval != nil {
			rows.Close()
			return dactfiles, errval
		}

		if strings.HasSuffix(s, ".dact") {
			// dactFileNameSplit := strings.FieldsFunc(dactfile, func(c rune) bool { return c == '/' || c == '\\' || c == '.' })
			// dactFileName := dactFileNameSplit[len(dactFileNameSplit)-2]
			// dactfiles = append(dactfiles, dactFileName)

			dactfiles = append(dactfiles, s)
		}
	}
	errval = rows.Err()
	return dactfiles, errval // probably nil
}

func gretelSendErr(message string, q *Context, err error) bool {
	if logerr(err) {
		http.Error(q.w, message+":"+err.Error(), 500)
		return true
	}

	return false
}

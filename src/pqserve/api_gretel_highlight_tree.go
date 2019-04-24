package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pebbe/dbxml"
)

// Does not actually highlight, but allows retrieving the full xml tree for a given sentence.
func api_gretel_highlight_tree(q *Context) {

	pathSegments := strings.Split(q.r.URL.EscapedPath(), "/")
	sentId := pathSegments[len(pathSegments)-1]
	dactFile := q.r.Form["db"][0]

	db, errval := dbxml.OpenRead(dactFile)
	if gretelSendErr("Cannot open database "+dactFile, q, errval) {
		return
	}

	xquery := xquery_gretel_highlight_tree(sentId)

	var qu *dbxml.Query
	qu, errval = db.Prepare(xquery, false, dbxml.Namespace{Prefix: "ud", Uri: "http://www.let.rug.nl/alfa/unidep/"})
	if gretelSendErr("Invalid query: "+xquery, q, errval) {
		return
	}

	docs, errval := qu.Run()
	if gretelSendErr("Cannot execute query: "+xquery, q, errval) {
		return
	}

	// read results
	docs.Next()
	result := docs.Match()

	if result == "" {
		gretelSendErr("Could not retrieve tree "+sentId, q, errors.New("Document not found"))
		return
	}

	q.w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")

	fmt.Fprint(q.w, result)
}

func xquery_gretel_highlight_tree(sentId string) string {
	return `
	for $match in collection() 
		where dbxml:metadata('dbxml:name', $match) = "` + sentId + `"
			return $match
	`
}

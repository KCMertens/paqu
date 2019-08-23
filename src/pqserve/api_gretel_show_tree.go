package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pebbe/dbxml"
)

// Does not actually highlight, but allows retrieving the full xml tree for a given sentence.
func api_gretel_show_tree(q *Context) {
	q.w.Header().Set("Access-Control-Allow-Origin", "*")

	// [${corpus}, ${dactfile}, ${sentenceid}]
	pathSegments := strings.Split(q.r.URL.EscapedPath(), "/")
	sentID := pathSegments[len(pathSegments)-1]
	dactFileID := pathSegments[len(pathSegments)-2]
	corpus := pathSegments[len(pathSegments)-3]

	if !mayAccess(q, corpus) {
		http.Error(q.w, "", 403)
		return
	}

	dactFile, errval := getDactFileById(q.db, corpus, dactFileID)
	if gretelSendErr("Error finding component "+dactFileID+" for corpus "+corpus, q, errval) {
		return
	}

	db, errval := dbxml.OpenRead(dactFile.path)
	if gretelSendErr("Error opening component database "+dactFile.path, q, errval) {
		return
	}

	xquery := xquery_gretel_show_tree(sentID)

	var qu *dbxml.Query
	qu, errval = db.PrepareRaw(xquery)
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
		gretelSendErr("Could not retrieve tree "+sentID, q, errors.New("Document not found"))
		return
	}

	q.w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")

	fmt.Fprint(q.w, result)
}

func xquery_gretel_show_tree(sentID string) string {
	return `
	for $match in collection() 
		where dbxml:metadata('dbxml:name', $match) = "` + sentID + `"
			return $match
	`
}

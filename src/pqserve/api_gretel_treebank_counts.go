// +build !nodbxml

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"

	"github.com/pebbe/dbxml"
)

type gretelCountsPayload struct {
	DactFiles *[]string `json:"components"`
	Corpus    string    `json:"corpus"`
	XPath     string    `json:"xpath"`
}

// In gretel4, the string key refers to a basex database (such as "LASSY_ID_WSU")
// String key for us referS to a dact file, using the full path on disk
// It's important this map contains all keys in the Components property of the configured_treebanks response (api_gretel_configured_treebanks)
type gretelCountsResponse map[string]int

func api_gretel_treebank_counts(q *Context) {

	requestBody, err := ioutil.ReadAll(q.r.Body)
	if gretelSendErr("Error reading request body", q, err) {
		return
	}

	var payload gretelCountsPayload
	err = json.Unmarshal(requestBody, &payload)
	if gretelSendErr("Invalid JSON in request body", q, err) {
		return
	}

	counts := make(gretelCountsResponse, 0)

	for _, dactFile := range *payload.DactFiles {
		db, errval := dbxml.OpenRead(dactFile)

		if gretelSendErr("Error opening database "+dactFile, q, errval) {
			return
		}

		xquery := createXquery(payload.XPath)

		var qu *dbxml.Query
		qu, errval = db.Prepare(xquery, false, dbxml.Namespace{Prefix: "ud", Uri: "http://www.let.rug.nl/alfa/unidep/"})
		if gretelSendErr("Invalid query "+xquery, q, errval) {
			return
		}

		count, errval := qu.Run()
		if gretelSendErr("Error executing query "+xquery, q, errval) {
			return
		}

		if !count.Next() {
			gretelSendErr("", q, count.Error())
			return
		}
		counts[dactFile], errval = strconv.Atoi(count.Value())

		if gretelSendErr("Invalid query result "+count.Match(), q, errval) {
			return
		}
	}

	q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")

	rbyte, errval := json.Marshal(counts)
	if gretelSendErr("Cannot marshal response", q, errval) {
		return
	}

	fmt.Fprint(q.w, string(json.RawMessage(rbyte)[:]))
}

func createXquery(xpath string) string {
	return "(count(collection()" + xpath + "))"
}

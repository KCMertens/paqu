// +build !nodbxml

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/pebbe/dbxml"
)

type gretelCountsPayload struct {
	DactFileIDs *[]string `json:"components"`
	Corpus      string    `json:"corpus"`
	XPath       string    `json:"xpath"`
}

// In gretel4, the string key refers to a basex database (such as "LASSY_ID_WSU")
// String key for us refers to a dact file, using the full path on disk
// It's important this map contains all keys in the Components property of the configured_treebanks response (api_gretel_configured_treebanks)
type gretelCountsResponse map[string]int

func api_gretel_treebank_counts(q *Context) {
	q.w.Header().Set("Access-Control-Allow-Origin", "*")

	requestBody, err := ioutil.ReadAll(q.r.Body)
	if gretelSendErr("Error reading request body", q, err) {
		return
	}

	var payload gretelCountsPayload
	err = json.Unmarshal(requestBody, &payload)
	if gretelSendErr("Invalid JSON in request body", q, err) {
		return
	}

	if !mayAccess(q, payload.Corpus) {
		http.Error(q.w, "", 403)
		return
	}

	counts := make(gretelCountsResponse, 0)

	for _, dactFileID := range *payload.DactFileIDs {
		dactFile, errval := getDactFileById(q.db, payload.Corpus, dactFileID)
		if gretelSendErr("Error finding component "+dactFileID+" for corpus "+payload.Corpus, q, errval) {
			return
		}

		db, errval := dbxml.OpenRead(dactFile.path)
		if gretelSendErr("Error opening component database "+dactFile.path, q, errval) {
			return
		}

		xquery := createXquery(payload.XPath)

		var qu *dbxml.Query
		qu, errval = db.PrepareRaw(xquery)
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
		counts[dactFile.id], errval = strconv.Atoi(count.Value())

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

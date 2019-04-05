// +build !nodbxml

package main

import (
	"errors"

	"github.com/pebbe/dbxml"

	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

func mini(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxi(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func unpack(s []string, vars ...*string) {
	for i, str := range s {
		*vars[i] = str
	}
}

type xPathVariable struct {
	// skip variable declaration in xquery if name == $node, that variable is already defined
	name string
	path string
}

type gretelResultsPayload struct {
	// The xpath expression to query the db
	XPath string `json:"xpath"`
	// TODO - Retrieve context around xpath results (or something like it)
	Context bool `json:"retrieveContext"`
	// corpus to search
	Corpus string `json:"corpus"`
	// variables to also output
	Variables []xPathVariable `json:"variables"`
	// the page that's being requested, maps to a set of results at a specific offset
	Page int `json:"iteration"`
	// Is this an analysis request, means larger result subsets are returned?
	Analysis bool `json:"isAnalysis"`

	// Set of unprocessed components - pingponged between client and server (except on first request
	//  in which case ComponentsToSearch is used)
	RemainingDactFiles *[]string `json:"remainingDatabases"`
	// subcomponents of the corpus to search, used to initialize RemainingComponents if present
	DactFilesToSearch []string `json:"components"`
}

// from gretel4 config.php
const searchLimit = 500
const analysisLimit = 50000
const searchPageSize = 50
const analysisPageSize = 50000

func api_gretel_results(q *Context) {
	requestBody, err := ioutil.ReadAll(q.r.Body)
	if gretelSendErr("Error reading request body", q, err) {
		return
	}

	var payload gretelResultsPayload
	err = json.Unmarshal(requestBody, &payload)
	if gretelSendErr("Invalid JSON in request body", q, err) {
		return
	}

	pageSize, searchLimit := func() (int, int) {
		if payload.Analysis {
			return analysisPageSize, analysisLimit
		}
		return searchPageSize, searchLimit
	}()

	// this is a bit dumb, but whatever.
	var remainingDactFiles = &payload.RemainingDactFiles
	if *remainingDactFiles == nil {
		files, err := getDactFiles(q.db, payload.Corpus)

		if gretelSendErr("", q, err) {
			return
		}
		*remainingDactFiles = &files
	}

	resultJSON, err := getResults(q, **remainingDactFiles, payload.Page, pageSize, searchLimit, payload.XPath, payload.Context, payload.Corpus, payload.Variables)
	if gretelSendErr("", q, err) {
		return
	}

	q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")

	fmt.Fprint(q.w, resultJSON)
}

// TODO use exceptions, error return code?
func getResults(q *Context, remainingDactFiles []string, page int, pageSize int, resultLimit int, xpath string, context bool, corpus string, variables []xPathVariable) (string, error) {
	startIndex := pageSize * page
	endIndex := mini(pageSize*(page+1), resultLimit)
	if startIndex >= endIndex || len(remainingDactFiles) == 0 {
		return "", errors.New("Out of bounds or no remaining databases to search")
	}
	dactFile := remainingDactFiles[0]
	sentenceMap := make(map[string]string)
	tbMap := make(map[string]string) // unused? only for grinded/sonar corpus?
	nodeIDMap := make(map[string]string)
	beginsMap := make(map[string]string)
	xmlSentencesMap := make(map[string]string)
	metaMap := make(map[string]string)
	variablesMap := make(map[string]string)
	originMap := make(map[string]string) // database where the sentence originated - we store the name of the dactfile here for now

	db, errval := dbxml.OpenRead(dactFile) // dactfile should be the full path, on the client we store this in the component.server_id field
	if errval != nil {
		return "", errors.New("Cannot open database " + dactFile)
	}

	xquery := xquery_gretel_results(startIndex, endIndex, xpath, context, variables)

	var qu *dbxml.Query
	qu, errval = db.Prepare(xquery, false, dbxml.Namespace{Prefix: "ud", Uri: "http://www.let.rug.nl/alfa/unidep/"})
	if errval != nil {
		return "", errors.New("Invalid query: " + xquery)
	}

	docs, errval := qu.Run()
	if errval != nil {
		return "", errors.New("Cannot execute query: " + xquery)
	}

	// read results
	matches := make(map[string]string)
	i := 0
	for docs.Next() {
		// docname := docs.Name()
		matches := strings.Split(docs.Match(), "</match>")
		for _, match := range matches {

			match = strings.TrimSpace(match)
			match = strings.Replace(match, "<match>", "", -1)
			if len(match) == 0 {
				continue
			}

			split := strings.Split(match, "||")

			// Add unique identifier to avoid overlapping sentences w/ same ID
			// Not entirely correct, endPos previously held endPosIteration (was page number? [endoffset / flushlimit])
			sentenceId := strings.TrimSpace(split[0]) + "-endPos=" + strconv.Itoa(endIndex) + "+match=" + strconv.Itoa(i) // usually empty (in testcorpus 'cdb' at least)
			sentence := strings.TrimSpace(split[1])
			nodeIds := split[2]
			begins := split[3]
			xmlSentences := split[4]
			meta := split[5]      // usually empty (in testcorpus 'cdb' at least)
			variables := split[6] // usually empty (only when variables requested by user)

			if len(sentence) == 0 || len(nodeIds) == 0 || len(begins) == 0 {
				continue
			}

			sentenceMap[sentenceId] = sentence
			nodeIDMap[sentenceId] = nodeIds
			beginsMap[sentenceId] = begins
			xmlSentencesMap[sentenceId] = xmlSentences
			metaMap[sentenceId] = meta
			variablesMap[sentenceId] = variables
			originMap[sentenceId] = dactFile

			i++
		}
	}

	// done with this dact file, remove it from the remaining files
	// and reset the page (it will be applied to the next dact file in the next request).
	if (startIndex + len(matches)) < endIndex {
		remainingDactFiles = remainingDactFiles[1:]
		page = -1 // We always increment current page by 1 so set to -1 to return page 0 to client
	}

	// TODO we should search across multiple dact files if we haven't found enough results to fill a page yet
	result := map[string]interface{}{
		"success":            true,
		"sentences":          sentenceMap,
		"tblist":             tbMap,
		"idlist":             nodeIDMap,
		"beginlist":          beginsMap,
		"xmllist":            xmlSentencesMap,
		"metalist":           metaMap,
		"varlist":            variablesMap,
		"endPosIteration":    page + 1,
		"databases":          remainingDactFiles,
		"sentenceDatabases":  originMap,
		"xquery":             xquery,
		"already":            nil,   // for grinded databases, which is not used in paqu
		"needRegularGrinded": false, // likewise
		"searchLimit":        resultLimit,
	}

	rbyte, errval := json.Marshal(result)
	if errval != nil {
		return "", errors.New("Cannot marshal response")
	}

	return string(json.RawMessage(rbyte)[:]), nil
}

/*
Example query from gretel4
--------------------------
(
    for $node in db:open("LASSY_ID_WRPE")/treebank//node[
        @cat="smain"
        and node[@rel="su" and @pt="vnw"]
        and node[@rel="hd" and @pt="ww"]
        and node[
            @rel="predc"
            and @cat="np"
            and node[@rel="det" and @pt="lid"]
            and node[@rel="hd" and @pt="n"]
        ]
    ]
        let $tree := ($node/ancestor::alpino_ds)
        let $sentid := ($tree/@id)
        let $sentence := ($tree/sentence)
        let $ids := ($node//@id)
        let $begins := ($node//@begin)
        let $beginlist := (distinct-values($begins))
        let $meta := ($tree/metadata/meta)

        return <match>{data($sentid)}||{data($sentence)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||</match>
)
[position() = 51 to 100]
*/
func xquery_gretel_results(startIndex int, endIndex int, xpath string, context bool, variables []xPathVariable) string {
	var variableDeclarations string
	var variableResults string
	for _, variable := range variables {
		variableResults += "<var name=\"" + variable.name + "\">{'" + variable.name + "'/@*}</var>"
		if variable.name != "$node" { // variable $node already exists in query
			variableDeclarations += "let " + variable.name + " := ('" + variable.path + "')[1]"
		}
	}

	// main node matching and iteration
	// xfor := "for $node in collection('"+dactfile+"')"+xpath
	xfor := "for $node in collection()" + xpath

	// Extract the following values for all matched nodes
	tree := " let $tree := ($node/ancestor::alpino_ds)"
	sentid := " let $sentid := ($tree/@id)" // it seems the .dact files do contain documents whose root is <alpino_ds> but there is no @id attribute...
	sentence := " let $sentence := ($tree/sentence)"
	ids := " let $ids := ($node//@id)"
	begins := " let $begins := ($node//@begin)"
	beginlist := " let $beginlist := (distinct-values($begins))"
	meta := " let $meta := ($tree/metadata/meta)"

	// only used when context == true
	prevs := " let $prevs := ($tree/preceding-sibling::alpino_ds[1]/sentence)"
	nexts := " let $nexts := ($tree/following-sibling::alpino_ds[1]/sentence)"

	// output of the xquery - print all the extracted variables
	var xquery string
	var xreturn string

	if context {
		xreturn = " return <match>{data($sentid)}||{data($prevs)} <em>{data($sentence)}</em> {data($nexts)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||" + variableResults + "</match>"
		xquery = xfor + tree + sentid + sentence + prevs + nexts + ids + begins + beginlist + meta + variableDeclarations + xreturn
	} else {
		xreturn = " return <match>{data($sentid)}||{data($sentence)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||" + variableResults + "</match>"
		xquery = xfor + tree + sentid + sentence + ids + begins + beginlist + meta + variableDeclarations + xreturn
	}

	// apply pagination parameters to the query
	xquery = "(" + xquery + ")[position() = " + strconv.Itoa(startIndex) + " to " + strconv.Itoa(endIndex) + "]"
	return xquery
}

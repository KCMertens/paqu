// +build !nodbxml

package main

import (
	"errors"

	"github.com/pebbe/dbxml"

	"encoding/json"
	"encoding/xml"
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
	Name string `json:"name"`
	Path string `json:"path"`
}

type gretelResultsPayload struct {
	// The xpath expression to query the db
	XPath string `json:"xpath"`
	// Retrieve the preceding and following sentences.
	// This appears to be UNSUPPORTED in dbxml
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

type innerXML struct {
	InnerXML string `xml:",innerxml"`
}

type gretelXqueryResult struct {
	SentenceID       string   `xml:"sentence-id"`
	Sentence         innerXML `xml:"sentence"`
	NodeBegins       []string `xml:"node-begins>id"`
	NodeIDs          []string `xml:"node-ids>id"`
	Result           innerXML `xml:"result"`
	Meta             innerXML `xml:"meta"`
	Variables        innerXML `xml:"vars"`
	PreviousSentence string   `xml:"prevous-sentence"`
	NextSentence     string   `xml:"next-sentence"`
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

	// This is a bit dumb, but whatever.
	// If the client didn't supply files to search, search them all.
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

func getResults(q *Context, remainingDactFiles []string, page int, pageSize int, resultLimit int, xpath string, context bool, corpus string, variables []xPathVariable) (string, error) {
	startIndex := (pageSize * page) + 1
	endIndex := mini(pageSize*(page+1), resultLimit)
	if startIndex >= endIndex || len(remainingDactFiles) == 0 {
		return "", errors.New("Out of bounds or no remaining databases to search")
	}

	dactFile := remainingDactFiles[0]
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

	sentenceMap := make(map[string]string)
	nodeIDMap := make(map[string]string)
	beginsMap := make(map[string]string)
	xmlSentencesMap := make(map[string]string)
	metaMap := make(map[string]string)
	variablesMap := make(map[string]string)
	originMap := make(map[string]string) // database where the sentence originated - we store the name of the dactfile here for now

	// read results
	i := 0
	for docs.Next() {
		var result gretelXqueryResult
		resultString := docs.Match()

		errval = xml.Unmarshal([]byte(resultString), &result)
		if errval != nil {
			return "", errval
		}

		sentenceID := result.SentenceID
		sentenceMap[sentenceID] = strings.TrimSpace(strings.Join([]string{result.PreviousSentence, result.Sentence.InnerXML, result.NextSentence}, " "))
		nodeIDMap[sentenceID] = strings.Join(result.NodeIDs, "-")
		beginsMap[sentenceID] = strings.Join(result.NodeBegins, "-")
		xmlSentencesMap[sentenceID] = result.Result.InnerXML
		metaMap[sentenceID] = result.Meta.InnerXML
		if result.Variables.InnerXML != "" {
			variablesMap[sentenceID] = "<vars>" + result.Variables.InnerXML + "</vars>" // client expects a certain data structure
		} else {
			variablesMap[sentenceID] = ""
		}
		originMap[sentenceID] = dactFile

		i++
	}

	// done with this dact file, remove it from the remaining files
	// and reset the page (it will be applied to the next dact file in the next request).
	if (startIndex + i) < endIndex {
		remainingDactFiles = remainingDactFiles[1:]
		page = -1 // We always increment current page by 1 so set to -1 to return page 0 to client
	}

	// TODO we should search across multiple dact files if we haven't found enough results to fill a page yet
	result := map[string]interface{}{
		// unused stuff, and meta info not extracted from the results directly
		"success":            true,
		"tblist":             make(map[string]string), // unused. Only for grinded corpora
		"databases":          remainingDactFiles,
		"endPosIteration":    page + 1,
		"xquery":             xquery,
		"already":            nil,   // for grinded databases, which is not used in paqu
		"needRegularGrinded": false, // likewise
		"searchLimit":        resultLimit,

		"sentences":         sentenceMap,
		"idlist":            nodeIDMap,
		"beginlist":         beginsMap,
		"xmllist":           xmlSentencesMap,
		"metalist":          metaMap,
		"varlist":           variablesMap,
		"sentenceDatabases": originMap,
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
	var optContextDeclaration string
	var optContextResults string
	if context {
		optContextDeclaration = `
			let $prevs := ($tree/preceding-sibling::alpino_ds[1]/sentence)
			let $nexts := ($tree/following-sibling::alpino_ds[1]/sentence)
		`

		optContextResults = `
			<previous-sentence>{data($prevs)}</previous-sentence>
			<next-sentence>{data($nexts)}</next-sentence>
		`
	}

	var optVariableDeclaration string
	var optVariableResults string
	for _, variable := range variables {
		optVariableResults += `<var name="` + variable.Name + `">{` + variable.Name + `/@*}</var>`

		if variable.Name != "$node" { // variable $node already exists in query
			optVariableDeclaration += "let " + variable.Name + " := (" + variable.Path + ")[1]\n"
		}
	}

	return `(
		for $node in collection()` + xpath + `
			let $tree := ($node/ancestor::alpino_ds)
			
			let $sentid := dbxml:metadata('dbxml:name', $tree)
			let $sentence := ($tree/sentence)
			let $begins := ($node//@begin)
			let $ids := (distinct-values($begins))
			let $meta := ($tree/metadata/meta)
	` + optVariableDeclaration + optContextDeclaration + `

		return 
			<match>
				<sentence-id>{data($sentid)}</sentence-id>
				<sentence><em>{data($sentence)}</em></sentence>
				<node-begins>
					{for $nodeId in $begins return <id>{data($nodeId)}</id>}
				</node-begins>
				<node-ids>
					{for $nodeId in $ids return <id>{data($nodeId)}</id>}
				</node-ids>
				<result>{$node}</result>
				<meta>{$meta}</meta>
				<vars>` + optVariableResults + `</vars>
				` + optContextResults + `
			</match>
	)[position() = ` + strconv.Itoa(startIndex) + ` to ` + strconv.Itoa(endIndex) + `]`
}

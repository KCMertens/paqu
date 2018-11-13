// +build !nodbxml

package main

import (
    "github.com/pebbe/dbxml"

    "fmt"
    "strconv"
    "strings"
    "encoding/json"
    "io/ioutil"
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

func unpack(s []string, vars... *string) {
    for i, str := range s {
        *vars[i] = str
    }
}

type XPathVariable struct {
    // skip variable declaration in xquery if name == $node, that variable is already defined 
    name    string 
    path    string
}

type RequestPayload struct {
    // The xpath expression to query the db
    XPath string                `json:"xpath"`
    // TODO - Retrieve context around xpath results (or something like it)
    Context bool                `json:"retrieveContext"`
    // corpus to search
    Corpus  string              `json:"corpus"`
    // variables to also output
    Variables []XPathVariable   `json:"variables"`
    // the page that's being requested, maps to a set of results at a specific offset
    Page int                    `json:"iteration"`
    // Is this an analysis request, means larger result subsets are returned?
    Analysis bool               `json:"isAnalysis"`

    // Set of unprocessed components - pingponged between client and server (except on first request
    //  in which case DactFiles is used)
    RemainingDactFiles *[]string `json:"remainingDatabases"`
    // subcomponents of the corpus to search
    DactFiles []string          `json:"components"`
    // It seems components is only used to populate remainingDatabases on the first request
    // then afterwards it's ignored and remainingdatabases is solely used.
    // This still leaves the case of the "iteration" int
    // what if we crossed into the next database halfway a page
    // then how do we know where to place the startindex, because it's not a multiple of the flushlimit (aka pagesize)
}

// from gretel4 config.php 
const resultsLimit = 500
const analysisLimit = 50000
// undefined in gretel4?
// const analysisFlushLimit := 

func api_gretel_results(q *Context) {
    var pageSize = 50
    var searchLimit = resultsLimit

    // TODO we're currently accessing through query parameters
    requestBody, err := ioutil.ReadAll(q.r.Body)
    if logerr(err) {
        return
    }
    
    var payload RequestPayload;
    err = json.Unmarshal(requestBody, &payload)
    if logerr(err) {
        return
    }

    if payload.Analysis {
        pageSize = analysisLimit
        searchLimit = analysisLimit
    }

    // this is a bit dumb, but whatever.
    var remainingDactFiles = payload.RemainingDactFiles
    if remainingDactFiles == nil {
        remainingDactFiles = &payload.DactFiles
    }

    resultJson := getResults(q, *remainingDactFiles, payload.Page, pageSize, searchLimit, payload.XPath, payload.Context, payload.Corpus, payload.Variables);
    
    q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
    q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")
	
    fmt.Fprint(q.w, resultJson)
    // if ff, ok := q.w.(http.Flusher); ok {
	// 	ff.Flush()
	// }
    // TODO
}

// TODO use exceptions, error return code?
func getResults(q *Context, remainingDactFiles []string, page int, pageSize int, resultLimit int, xpath string, context bool, corpus string, variables []XPathVariable) string {
    startIndex := pageSize * page;
    endIndex := mini(pageSize * (page + 1), resultLimit)
    if startIndex >= endIndex || len(remainingDactFiles) == 0 {
        return "Out of bounds or no remaining databases to search"; // TODO/empty response 
    }
    dactFile := remainingDactFiles[0]
    dactFileNameSplit := strings.FieldsFunc(dactFile, func(c rune) bool { return c == '/' || c == '\\' || c == '.' })
    dactFileName := dactFileNameSplit[len(dactFileNameSplit)-2]
            
    
    sentenceMap := make(map[string]string)
    tbMap := make(map[string]string) // unused? only for grinded/sonar corpus?
    nodeIdMap := make(map[string]string)
    beginsMap := make(map[string]string)
    xmlSentencesMap := make(map[string]string)
    metaMap := make(map[string]string)
    variablesMap := make(map[string]string)
    originMap := make(map[string]string) // database where the sentence originated - we store the name of the dactfile here for now

    db, errval := dbxml.OpenRead(dactFile) // dactfile should be the full path, on the client we store this in the component.server_id field
    if logerr(errval) {
        return "Cannot open database " + dactFile
    }

    xquery := createXquery(startIndex, endIndex, xpath, context, variables)

    var qu *dbxml.Query
    qu, errval = db.Prepare(xquery, false, dbxml.Namespace{Prefix: "ud", Uri: "http://www.let.rug.nl/alfa/unidep/"})
    if logerr(errval) {
        return "Invalid query " + xquery
    }
    
    docs, errval := qu.Run()
    if logerr(errval) {
        return "Cannot execute query " + xquery
    }

    // read results
    var matches []string
    for docs.Next() {
        // const name = docs.Name()
        // const content = docs.Content()
        // const match = docs.Match()

        matches = append(matches, strings.Split(docs.Match(), "</match>")...)
    }

    // Process results

    for i, match := range matches {
        match = strings.TrimSpace(match)
        match = strings.Replace(match, "<match>", "", -1)
        if len(match) == 0 {
            continue
        }

        split := strings.Split(match, "||")
        
        // Add unique identifier to avoid overlapping sentences w/ same ID
        // Not entirely correct, endPos previously held endPosIteration (was page number? [endoffset / flushlimit])
        sentenceId := strings.TrimSpace(split[0])+"-endPos="+strconv.Itoa(endIndex)+"+match="+strconv.Itoa(i) // usually empty (in testcorpus 'cdb' at least)
        sentence := strings.TrimSpace(split[1])
        nodeIds := split[2]
        begins := split[3]
        xmlSentences := split[4]
        meta := split[5] // usually empty (in testcorpus 'cdb' at least)
        variables := split[6] // usually empty (only when variables requested by user)
        
        if len(sentence) == 0 || len(nodeIds) == 0 || len(begins) == 0 {
            continue
        }
        
        sentenceMap[sentenceId] = sentence
        nodeIdMap[sentenceId] = nodeIds
        beginsMap[sentenceId] = begins
        xmlSentencesMap[sentenceId] = xmlSentences 
        metaMap[sentenceId] = meta
        variablesMap[sentenceId] = variables
        originMap[sentenceId] = dactFileName
    }

    // done with this dact file, remove it from the remaining files
    // and reset the page (it will be applied to the next dact file in the next request).
    if len(matches) < (endIndex - startIndex) { 
        remainingDactFiles = remainingDactFiles[1:]
        page = -1 // We always increment current page by 1 so set to -1 to return page 0 to client
    }

    // TODO we should search across multiple dact files if we haven't found enough results to fill a page yet
    result := []interface{}{sentenceMap, tbMap, nodeIdMap, beginsMap, xmlSentencesMap, metaMap, variablesMap, page+1, remainingDactFiles, originMap, xquery}
    
    rbyte, errval := json.Marshal(result)
    if logerr(errval) {
        return "Cannot marshal response"
    }
    rbyte = json.RawMessage(rbyte)
    if logerr(errval) {
        return "Cannot encode response"
    }
    
    return string(rbyte[:])
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
func createXquery(startIndex int, endIndex int, xpath string, context bool, variables []XPathVariable) string {
    var variable_declarations string;
    var variable_results string;
    for _, variable := range variables {
        variable_results += "<var name=\""+variable.name+"\">{'"+variable.name+"'/@*}</var>"
        if variable.name != "$node" { // variable $node already exists in query
            variable_declarations += "let "+variable.name+" := ('"+variable.path+"')[1]"
        }
    }
    
    // main node matching and iteration
    // xfor := "for $node in collection('"+dactfile+"')"+xpath
    xfor := "for $node in collection()"+xpath

    // Extract the following values for all matched nodes
    tree :=     " let $tree := ($node/ancestor::alpino_ds)"
    sentid :=   " let $sentid := ($tree/@id)" // it seems the .dact files do contain documents whose root is <alpino_ds> but there is no @id attribute...
    sentence := " let $sentence := ($tree/sentence)"
    ids :=      " let $ids := ($node//@id)"
    begins :=   " let $begins := ($node//@begin)"
    beginlist :=" let $beginlist := (distinct-values($begins))"
    meta :=     " let $meta := ($tree/metadata/meta)"
    
    // only used when context == true
    prevs   := " let $prevs := ($tree/preceding-sibling::alpino_ds[1]/sentence)"
    nexts   := " let $nexts := ($tree/following-sibling::alpino_ds[1]/sentence)"
    
    // output of the xquery - print all the extracted variables
    var xquery string
    var xreturn string
    
    if (context) {
        xreturn = " return <match>{data($sentid)}||{data($prevs)} <em>{data($sentence)}</em> {data($nexts)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||"+variable_results+"</match>"
        xquery = xfor+tree+sentid+sentence+prevs+nexts+ids+begins+beginlist+meta+variable_declarations+xreturn;
    } else {
        xreturn = " return <match>{data($sentid)}||                   {data($sentence)}                    ||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||"+variable_results+"</match>"
        xquery = xfor+tree+sentid+sentence+            ids+begins+beginlist+meta+variable_declarations+xreturn
    }

    // apply pagination parameters to the query
    xquery = "("+xquery+")[position() = "+strconv.Itoa(startIndex)+" to "+strconv.Itoa(endIndex)+"]"
    return xquery
}

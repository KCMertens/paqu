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

func min(a int, b int) int {
    if a < b { 
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
    XPath string                `json:field_xpath`
    // TODO - Retrieve context around xpath results (or something like it)
    Context bool                `json:field_retrieveContext`
    // corpus to search
    Corpus  string              `json:field_corpus`
    // variables to also output
    Variables []XPathVariable   `json:field_variables`
    // the page that's being requested, maps to a set of results at a specific offset
    Iteration int               `json:field_iteration`
    // Is this an analysis request, means larger result subsets are returned?
    Analysis bool               `json:field_isAnalysis`

    // Unused - todo investigate whether (and how) to mock these fields in the response
    // Relevance within gretel4 not entirely known yet
    // set of unprocessed components - pingponged between client and server?
    RemainingDatabases []string `json:field_remainingDatabases`
    // subcomponents of the corpus to search
    Components []string         `json:field_components`
}

func api_gretel_results(q *Context) {
    // from gretel4 config.php 
    const resultsLimit = 500
    const analysisLimit = 50000
    // undefined in gretel4?
    // const analysisFlushLimit := 
    var flushLimit = 50
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
        flushLimit = analysisLimit
        searchLimit = analysisLimit
    }

    startOffset := min(payload.Iteration * flushLimit, searchLimit)
    endOffset := min((payload.Iteration+1) * flushLimit, searchLimit)
    if startOffset >= searchLimit {
        return; // TODO proper empty response 
    }

    resultJson := getResults(q, startOffset, endOffset, payload.XPath, payload.Context, payload.Corpus, payload.Variables);
    
    q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
    q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")
	
    fmt.Fprint(q.w, resultJson)
    // if ff, ok := q.w.(http.Flusher); ok {
	// 	ff.Flush()
	// }
    // TODO
}

func getResults(q *Context, startOffset int, endOffset int, xpath string, context bool, corpus string, variables []XPathVariable) string {
    dactfiles := make([]string, 0)
    rows, errval := q.db.Query(fmt.Sprintf("SELECT `arch` FROM `%s_c_%s_arch` ORDER BY `id`", Cfg.Prefix, corpus))
    if logerr(errval) {
        return ""
    }
    for rows.Next() {
        var s string
        errval = rows.Scan(&s)
        if logerr(errval) {
            rows.Close()
            return ""
        }
        if strings.HasSuffix(s, ".dact") {
            dactfiles = append(dactfiles, s)
        }
    }
    errval = rows.Err()
    if logerr(errval) {
        return ""
    }

    if len(dactfiles) == 0 {
        fmt.Fprintln(q.w, "Er zijn geen dact-bestanden voor dit corpus")
        return ""
    }
    

    for _, dactfile := range dactfiles {
        db, errval := dbxml.OpenRead(dactfile)
        if logerr(errval) {
            return ""
        }

		// xquery := "(for $node in //node[ @cat=\"smain\" and node[@rel=\"su\" and @pt=\"vnw\"] and node[@rel=\"hd\" and @pt=\"ww\"] and node[@rel=\"predc\" and @cat=\"np\" and node[@rel=\"det\" and @pt=\"lid\"] and node[@rel=\"hd\" and @pt=\"n\"]]] let $tree := ($node/ancestor::alpino_ds) let $sentid := ($tree/@id) let $sentence := ($tree/sentence) let $ids := ($node//@id) let $begins := ($node//@begin) let $beginlist := (distinct-values($begins)) let $meta := ($tree/metadata/meta) return <match>{data($sentid)}||{data($sentence)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||</match>)[position() >= 1 to 50]"
        xquery := createXquery(dactfile, startOffset, endOffset, xpath, context, variables)
    
        var qu *dbxml.Query
        qu, errval = db.Prepare(xquery, false, dbxml.Namespace{Prefix: "ud", Uri: "http://www.let.rug.nl/alfa/unidep/"})
        if logerr(errval) {
            return ""
        }
        
        docs, errval := qu.Run()
        if logerr(errval) {
            return ""
        }

        // read results
        var matches []string
        for docs.Next() {
            // const name = docs.Name()
            // const content = docs.Content()
            // const match = docs.Match()

            matches = append(matches, strings.Split(docs.Match(), "</match>")...)

            // now process match? this will be stupid
            // let's first attempt to just echo the matches
        }

        sentenceMap := make(map[string]string)
        tbMap := make(map[string]string) // unused? only for grinded/sonar corpus?
        nodeIdMap := make(map[string]string)
        beginsMap := make(map[string]string)
        xmlSentencesMap := make(map[string]string)
        metaMap := make(map[string]string)
        variablesMap := make(map[string]string)
        originMap := make(map[string]string) // database where the sentence originated - we store the dactfile here for now

        // Process results

        // varsRegex, errval := regexp.Compile("<vars>.*</vars>/s")
        // if logerr(errval) {
        //     return ""
        // }

        for i, match := range matches {
            match = strings.TrimSpace(match)
            match = strings.Replace(match, "<match>", "", -1)
            if len(match) == 0 {
                continue
            }

            split := strings.Split(match, "||")
            
            // Add unique identifier to avoid overlapping sentences w/ same ID
            // Not entirely correct, endPos previously held endPosIteration (was page number? [endoffset / flushlimit])
            sentenceId := strings.TrimSpace(split[0])+"-endPos="+strconv.Itoa(endOffset)+"+match="+strconv.Itoa(i) // usually empty (in testcorpus 'cdb' at least)
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
            originMap[sentenceId] = dactfile
        }

        result := []interface{}{sentenceMap, tbMap, nodeIdMap, beginsMap, xmlSentencesMap, metaMap, variablesMap, endOffset, "[]" /* remaining basex databases to process, not applicable */, originMap, xquery}
        
        rbyte, errval := json.Marshal(result)
        if logerr(errval) {
            return ""
        }
        rbyte = json.RawMessage(rbyte)
        if logerr(errval) {
            return ""
        }
        

        return string(rbyte[:])
        
        // fmt.println(q.w, json)
        // return resultJson

        // TODO maybe store remaining dact files in the remaining databases? 
        // also continue across dact file borders if we haven't yet processed enough results for our window
        // TODO acquire some test data using multiple dact files.
        // also what to do with dactx data?
    }
    return "" // TODO when no files that should probably signal an error (there might be other issues before here with missing mysql tables or something)

    // $results = getSentences($corpus, $databases, $already, $start, $session, null, $searchLimit, $xpath, $context, $variables);
    // if ($results[7] * $flushLimit >= $searchLimit) {
    //     // clear the remaining databases to signal the search is done
    //     $results[8] = 
    // $session->close();array();
    // }
    // return $results;
}

/**
 * @param $variables An array with variables to return. Each element should contain name and path.
 */
 /*
func getSentences(corpus string, endPosIteration int, searchLimit int, xpath string, context bool, variables []XPathVariable) {
    var matchesAmount := 0;
        
    for {
        ++endPosIteration;          
        
        const xquery := createXquery(endPosIteration, searchLimit, $flushLimit, $needRegularSonar, corpus, $components, $context, $xpath, $variables);
        $query = $session->query($xquery);
        $result = $query->execute();
        $query->close();
        if (!$result || $result == 'false') {
            if ($endPosIteration !== 'all') {
                // go to the next database and start at the first position of that
                $endPosIteration = 0;
            }
            break;
        }
        $matches = explode('</match>', $result);
        $matches = array_cleaner($matches);
        while ($match = array_shift($matches)) {
            if ($endPosIteration === 'all' && $matchesAmount >= $searchLimit) {
                break 3;
            }
            $match = str_replace('<match>', '', $match);
            if ($corpus == 'sonar') {
                list($sentid, $sentence, $tb, $ids, $begins) = explode('||', $match);
            } else {
                list($sentid, $sentence, $ids, $begins, $xml_sentences, $meta) = explode('||', $match);
            }
            if (isset($sentid, $sentence, $ids, $begins)) {
                ++$matchesAmount;
                $sentid = trim($sentid);
                // Add unique identifier to avoid overlapping sentences w/ same ID
                $sentid .= '-endPos='.$endPosIteration.'+match='.$matchesAmount;
                $sentences[$sentid] = $sentence;
                $idlist[$sentid] = $ids;
                $beginlist[$sentid] = $begins;
                $xmllist[$sentid] = $xml_sentences;
                $metalist[$sentid] = $meta;
                preg_match('/<vars>.*<\/vars>/s', $match, $varMatches);
                $varList[$sentid] = count($varMatches) == 0 ? '' : $varMatches[0];
                if ($corpus == 'sonar') {
                    $tblist[$sentid] = $tb;
                }
                $sentenceDatabases[$sentid] = corpus;
            }
        }
        if ($endPosIteration === 'all') {
            break;
        } elseif ($matchesAmount >= $flushLimit) {
            // Re-add pop'd database because it is very likely we aren't finished with it
            // More results are still in that database but because of the flushlimit we
            // have to bail out
            $databases[] = corpus;
            break 2;
        }
    }
        
    if (isset($sentences)) {
        if ($endPosIteration !== 'all') {
            if ($sid != null) {
                session_start();
                $_SESSION[$sid]['endPosIteration'] = $endPosIteration;
                $_SESSION[$sid]['flushDatabases'] = $databases;
                $_SESSION[$sid]['flushAlready'] = $already;
                session_write_close();
            }
        }
        if ($corpus !== 'sonar') {
            $tblist = false;
        }
        return array($sentences, $tblist, $idlist, $beginlist, $xmllist, $metalist, $varList, $endPosIteration, $databases, $sentenceDatabases, $xquery);
    } else {
        // in case there are no results to be found
        return false;
    }
}
*/

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
func createXquery(dactfile string, startIndex int, endIndex int, xpath string, context bool, variables []XPathVariable) string {
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
    
    // TODO
//     if ($context) {
//         if ($corpus == 'sonar' && !$needRegularSonar) {
//             $databases = $component[0].'sentence2treebank';
//             $text = 'let $text := fn:replace($sentid[1], \'(.+?)(\d+)$\', \'$1\')';
//             $snr = 'let $snr := fn:replace($sentid[1], \'(.+?)(\d+)$\', \'$2\')';
//             $prev = 'let $prev := (number($snr)-1)';
//             $next = 'let $next := (number($snr)+1)';
//             $previd = 'let $previd := concat($text, $prev)';
//             $nextid = 'let $nextid := concat($text, $next)';
//             $prevs = 'let $prevs := (db:open("'.$databases.'")';
//             $nexts = 'let $nexts := (db:open("'.$databases.'")';
//             if ($corpus != 'sonar') {
//                 $prevs .= '//s[id=$previd]/sentence)';
//                 $nexts .= '//s[id=$nextid]/sentence)';
//             } else {
//                 $prevs .= '/sentence2treebank/sentence[@nr=$previd])';
//                 $nexts .= '/sentence2treebank/sentence[@nr=$nextid])';
//             }
//             $return = ' return <match>{data($sentid)}||{data($prevs)} <em>{data($sentence)}</em> {data($nexts)}'
//             .$returnTb.'||{string-join($ids, \'-\')}||{string-join($beginlist, \'-\')}||'.$variable_results.'</match>';
//             $xquery = $for.$xpath.PHP_EOL.$tree.$sentid.$sentence.$ids.$begins.$beginlist.$text.$snr.$prev.$next.$previd.$nextid.$prevs.$nexts.$variable_declarations.$return;
//         } else {
//             $context_sentences = 'let $prevs := ($tree/preceding-sibling::alpino_ds[1]/sentence)
// let $nexts := ($tree/following-sibling::alpino_ds[1]/sentence)';
//             $return = ' return <match>{data($sentid)}||{data($prevs)} <em>{data($sentence)}</em> {data($nexts)}'.$returnTb
//                 .'||{string-join($ids, \'-\')}||{string-join($beginlist, \'-\')}||{$node}||{$meta}||'.$variable_results.'</match>';
//             $xquery = $for.$xpath.PHP_EOL.$tree.$sentid.$sentence.$context_sentences.$regulartb.$ids.$begins.$beginlist.$meta.$variable_declarations.$return;
//         }
//     } else {
        xreturn := " return <match>{data($sentid)}||{data($sentence)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||"+variable_results+"</match>"
        
        // xquery := xfor+tree+sentid+sentence+ids+begins+beginlist+meta+variable_declarations+xreturn;
        xquery := xfor+tree+sentid+sentence+ids+begins+beginlist+meta+variable_declarations+xreturn;
    // }

    // return xquery
    xquery = "("+xquery+")[position() = "+strconv.Itoa(startIndex)+" to "+strconv.Itoa(endIndex)+"]"
    return xquery
}
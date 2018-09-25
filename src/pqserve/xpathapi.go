// +build !nodbxml

package main

import (
    "github.com/pebbe/dbxml"

    "bytes"
    "crypto/md5"
    "database/sql"
    "encoding/xml"
    "errors"
    "fmt"
    "html"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"
    "time"
    "encoding/json"
    "regexp"
)

type XPathVariable struct {
    // skip variable declaration in xquery if name == $node, that variable is already defined 
    name    string 
    path    string
}


/** TODO document request payload */
func xpathapi(q *Context) {
    // from gretel4 config.php 
    const resultsLimit := 500
    const analysisLimit := 50000
    // undefined in gretel4?
    // const analysisFlushLimit := 
    var flushLimit := 50
    var searchLimit := resultsLimit

    // TODO we're currently accessing through query parameters
    // we need to use the POST body as json.
    
    // string - the xpath expression
    const xpath := first(q.r, "xpath") 
    // [bool] (false) - get context around hits
    const context := first(q.r, "retrieveContext") 
    // string- the index we're searching in
    const corpus := first(q.r, "corpus")
    // []string - not sure if components are a thing in paqu, mostly 2-letter ids in basex
    // different for grinded corpora, but that's not a thing in paqu
    // const components := first(q.r, "components")
    // []XPathVariable - xpath expressions whose results are stored in a named variable in the output
    // nil if not passed.
    const variables := first(q.r, "variables")
    // int - request page, actual results depending on flushlimit and resultlimit
    const iteration := first(q.r, "iteration")
    // []string set of unprocessed components - pingponged between client and server?
    // const remainingDatabases := first(q.r, "remainingDatabases")
    
    // [bool] (false) - not always passed
    const isAnalysis := first(q.r, "isAnalysis")
    if isAnalysis {
        flushLimit = analysisLimit
        searchLimit = analysisLimit
    }

    const startOffset = min(iteration * flushLimit, searchLimit)
    const endOffset = min(iteration+1 * flushLimit, searchLimit)
    if startOffset >= searchLimit {
        return; // TODO proper empty response 
    }

    const results = getResults(q, startOffset, endOffset, xpath, context, corpus, variables);


    
    // TODO
    // header('Content-Type: application/json');
    // echo json_encode($results);
}

func getResults(q *Context, startOffset int, endOffset int, xpath string, context bool, corpus string, components []string, start int, searchLimit int, variables []XPathVariable)
{
    dactfiles := make([]string, 0)
	global := true
    rows, errval = q.db.Query(fmt.Sprintf("SELECT `arch` FROM `%s_c_%s_arch` ORDER BY `id`", Cfg.Prefix, corpus))
    if logerr(errval) {
        return
    }
    for rows.Next() {
        var s string
        errval = rows.Scan(&s)
        if logerr(errval) {
            rows.Close()
            return
        }
        if strings.HasSuffix(s, ".dact") {
            dactfiles = append(dactfiles, s)
        }
    }
    errval = rows.Err()
    if logerr(errval) {
        return
    }

    if len(dactfiles) == 0 {
        fmt.Fprintln(q.w, "Er zijn geen dact-bestanden voor dit corpus")
        return
    }
    

    for i, dactfile := range dactfiles {
        db, errval = dbxml.OpenRead(dactfile)
        if logerr(errval) {
            return
        }

        const xquery = createXquery(startOffset, endOffset, xpath, context, variables)
    
        var qu *dbxml.Query
        qu, errval = db.Prepare(xquery, dbxml.Namespace{Prefix: "ud", Uri: "http://www.let.rug.nl/alfa/unidep/"})
        if logerr(errval) {
            return
        }
        interrupted := make(chan bool, 1)
        go func() {
            select {
            case <-chClose:
                interrupted <- true
                logerr(errConnectionClosed)
                qu.Cancel()
            }
        }()
    
        docs, errval = qu.Run()
        if logerr(errval) {
            return
        }


        
        // read results
        var matches []string
        for docs.Next() {
            const name = docs.Name()
            const content = docs.Content()
            const match = docs.Match()

            matches = append(matches, strings.split(match, "</result>"))

            // now process match? this will be stupid
            // let's first attempt to just echo the matches
        }

        var sentenceMap := make(map[string]string)
        var tbMap := make(map[string]string) // unused? only for grinded/sonar corpus?
        var nodeIdMap := make(map[string]string)
        var beginsMap := make(map[string]string)
        var xmlSentencesMap := make(map[string]string)
        var metaMap := make(map[string]string)
        var variablesMap := make(map[string]string)
        var originMap := make(map[string]string) // database where the sentence originated - we store the dactfile here for now

        // Process results

        const varsRegex := regex.Compile("<vars>.*<\/vars>/s")
        for i, match := range matches {
            match = strings.TrimSpace(match)
            match = strings.Replace(match, "<result>", "", -1)
            if match.Len() == 0 {
                continue
            }

            var sentenceId, sentence, nodeIds, begins, xmlSentences, meta string
            unpack(strings.split(match, "||"), &sentenceId, &sentence, &nodeIds, &begins, &xmlSentences, &meta)

            if strings.Len(strings.Trim(sentenceId)) == 0 || 
               strings.Len(sentence) == 0 || 
               strings.Len(ids) == 0 ||
               strings.Len(begins) == 0 
            {
                continue
            }

            // Add unique identifier to avoid overlapping sentences w/ same ID
            // Not entirely correct, endPos previously held endPosIteration (was page number? [endoffset / flushlimit])
            sentenceId = strings.Trim(sentenceId)+"-endPos="+endOffset+"+match="+i 
            
            sentenceMap[sentenceId] = sentence
            nodeIdMap[sentenceId] = nodeIds
            beginsMap[sentenceId] = begins
            xmlSentencesMap[sentenceId] = xmlSentences 
            metaMap[sentenceId] = meta
            variablesMap[sentenceId] = varsRegex.Find(match)
            originMap[sentenceId] = dactfile
        }

        var result := []interface{}{sentenceMap, tbMap, nodeIdMap, beginsMap, xmlSentencesMap, metaMap, variablesMap, endoffset, "[]" /* remaining basex databases to process, not applicable */, originMap, xquery}
        
        json, errval = json.Marshal(result)
        if logerr(errval) {
            return
        }

        fmt.println(q.w, json)
        return


        // TODO maybe store remaining dact files in the remaining databases? 
        // also continue across dact file borders if we haven't yet processed enough results for our window
        // TODO acquire some test data using multiple dact files.
        // also what to do with dactx data?
    }



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
func createXquery(startIndex int, endIndex int, xpath string, context bool, variables: []XPathVariable) {
    var variable_declarations string;
    var variable_results string;
    for i, variable := range variables {
        variable_results += "<var name=\""+variable.name"\">{'"+variable.name+"'/@*}</var>:"
        if variable.name != "$node" { // variable $node already exists in query
            variable_declarations += "let "+variable.name+" := ('"+value.path+"')[1]"
        }
    }
    
    // main node matching and iteration
    var xfor := "for $node in "+xpath

    // Extract the following values for all matched nodes (not sure if this will even work, assume the xml is similar?)
    var tree :=     "let $tree := ($node/ancestor::alpino_ds)"
    var sentid :=   "let $sentid := ($tree/@id)"
    var sentence := "let $sentence := ($tree/sentence)"
    var ids :=      "let $ids := ($node//@id)"
    var begins :=   "let $begins := ($node//@begin)"
    var beginlist = "let $beginlist := (distinct-values($begins))"
    var meta :=     "let $meta := ($tree/metadata/meta)"
    
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
        const xreturn := " return <match>{data($sentid)}||{data($sentence)}||{string-join($ids, '-')}||{string-join($beginlist, '-')}||{$node}||{$meta}||"+variable_results+"</match>"
        
        xquery = xfor+xpath+'\n'+tree+sentid+sentence+ids+begins+beginlist+meta+variable_declarations+xreturn;
    // }

    xquery = "("+xquery+")[position() ="+startIndex+" to "+endIndex+"]"
    return $xquery;
}
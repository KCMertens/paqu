// +build !nodbxml

package main

import (
//     "github.com/pebbe/dbxml"

    "fmt"
//     "strconv"
    "strings"
    "encoding/json"
//     "io/ioutil"
)

type TreebankComponent struct {
    Database_id string      `json:"database_id"`
    Description string      `json:"description"`
    Sentences interface{}   `json:"sentences"` // number if known, else "?"
    Title string            `json:"title"`
    Words interface{}       `json:"words"` // number if known, else "?"
}

type TreebankMetadata struct {
    Field string    `json:"field"`
    Type string     `json:"type"` // 'text' | 'int' | 'date',
    Facet string    `json:"facet"` // 'checkbox' | 'slider' | 'range' | 'dropdown',
    Show bool       `json:"show"`
    
    //minValue?: number | Date,
    //maxValue?: number | Date
}

type Treebank struct {
    Components map[string]TreebankComponent `json:"components"`
    Description string                      `json:"description"`
    Title string                            `json:"title"`
    Metadata []TreebankMetadata             `json:"metadata"`
}

type ConfiguredTreebanksResponse map[string]Treebank

func api_gretel_configured_treebanks(q *Context) {

    treebanks := make(map[string]Treebank)
    TREEBANKS: 
    for id, _ := range q.prefixes {
       
        dactfiles := make([]string, 0)
        rows, errval := q.db.Query(fmt.Sprintf("SELECT `arch` FROM `%s_c_%s_arch` ORDER BY `id`", Cfg.Prefix, id))
        if logerr(errval) {
            continue TREEBANKS
        }
        for rows.Next() {
            var s string
            errval = rows.Scan(&s)
            if logerr(errval) {
                rows.Close()
                continue TREEBANKS
            }
            if strings.HasSuffix(s, ".dact") {
                dactfiles = append(dactfiles, s)
            }
        }
        errval = rows.Err()
        if logerr(errval) {
            continue TREEBANKS
        }
        
        treebanks[id] = Treebank{
            // TODO list .dact files as components. retrieve number of sentences and words from the file if possible?
            // might be a simple query we can do to get this.
            Components: make(map[string]TreebankComponent),
            Description: "Todo",
            Title: id,
            Metadata: make([]TreebankMetadata, 0),
        }
    
        for _, dactfile := range dactfiles {
            dactFileNameSplit := strings.FieldsFunc(dactfile, func(c rune) bool { return c == '/' || c == '\\' || c == '.' })
            dactFileName := dactFileNameSplit[len(dactFileNameSplit)-2]
    
            treebanks[id].Components[dactfile] = TreebankComponent{
                Database_id: dactFileName,
                Description: dactFileName,
                Sentences: "?",
                Title: dactFileName,
                Words: "?",
            }
        }
    }

    // TODO error handling
    rbyte, errval := json.Marshal(treebanks)
    if logerr(errval) {
        return
    }
    rbyte = json.RawMessage(rbyte)
    if logerr(errval) {
        return
    }

    q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
    q.w.Header().Set("Cache-Control", "no-cache")
    q.w.Header().Add("Pragma", "no-cache")

    fmt.Fprint(q.w, string(rbyte[:]))
}
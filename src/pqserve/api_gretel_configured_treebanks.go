// +build !nodbxml

package main

import (
//     "github.com/pebbe/dbxml"

    "fmt"
//     "strconv"
//     "strings"
    "encoding/json"
//     "io/ioutil"
)

type TreebankComponent struct {
    Database_id string  `json:"database_id"`
    Description string  `json:"description"`
    Sentences int       `json:"sentences"`
    Title string        `json:"title"`
    Words int           `json:"words"`
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
    // TODO we're currently accessing through query parameters

    treebanks := make(map[string]Treebank)
    for id, _ := range q.prefixes {
        treebanks[id] = Treebank{
            Components: map[string]TreebankComponent{
                "default": TreebankComponent{
                    Database_id: "",
                    Description: "Search this whole corpus",
                    Sentences: 1,
                    Title: "Everything",
                    Words: 1,
                },
            },
            Description: "Todo",
            Title: id,
            Metadata: make([]TreebankMetadata, 0),
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
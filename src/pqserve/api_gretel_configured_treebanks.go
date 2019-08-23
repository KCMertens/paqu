// +build !nodbxml

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type gretelTreebankComponent struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Title       string      `json:"title"`
	Sentences   interface{} `json:"sentences"` // number if known, else "?"
	Words       interface{} `json:"words"`     // number if known, else "?"
}

type gretelTreebankMetadata struct {
	Field string `json:"field"`
	Type  string `json:"type"`  // 'text' | 'int' | 'date',
	Facet string `json:"facet"` // 'checkbox' | 'slider' | 'range' | 'dropdown',
	Show  bool   `json:"show"`

	//minValue?: number | Date,
	//maxValue?: number | Date
}

type gretelTreebank struct {
	Components  map[string]gretelTreebankComponent `json:"components"`
	Description string                             `json:"description"`
	Title       string                             `json:"title"`
	Metadata    []gretelTreebankMetadata           `json:"metadata"`
}

type gretelConfiguredTreebanksResponse map[string]gretelTreebank

func api_gretel_configured_treebanks(q *Context) {
	q.w.Header().Set("Access-Control-Allow-Origin", "*")

	treebanks := make(map[string]gretelTreebank)
TREEBANKS:
	for id := range q.prefixes {
		dactfiles, errval := getDactFiles(q.db, id)
		if errval != nil {
			continue TREEBANKS
		}

		treebanks[id] = gretelTreebank{
			Components:  make(map[string]gretelTreebankComponent),
			Description: q.desc[id],
			Title:       id,
			Metadata:    make([]gretelTreebankMetadata, 0),
		}

		for _, dactfile := range dactfiles {
			dactFileNameSplit := strings.FieldsFunc(dactfile.path, func(c rune) bool { return c == '/' || c == '\\' || c == '.' })
			dactFileName := dactFileNameSplit[len(dactFileNameSplit)-2]

			treebanks[id].Components[dactfile.id] = gretelTreebankComponent{
				ID:          dactfile.id,
				Description: dactFileName,
				Sentences: (func() interface{} {
					if len(dactfiles) == 1 && q.lines[id] > 0 {
						return q.lines[id]
					} else {
						return "?"
					}
				})(),
				Title: dactFileName,
				Words: (func() interface{} {
					if len(dactfiles) == 1 && q.words[id] > 0 {
						return q.words[id]
					}
					return "?"
				})(),
			}
		}
	}

	// TODO error handling
	rbyte, errval := json.Marshal(treebanks)
	if gretelSendErr("Error encoding response", q, errval) {
		return
	}

	q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")

	fmt.Fprint(q.w, string(json.RawMessage(rbyte)[:]))
}

// This version is nicer,
// we subdivide the sentences based on their metadata, and return them as components
// but when requests come in to retrieve results, we don't know which dact file they came from.
// So to use it, gretel_results will need to be edited to reverse map that info
// which is more work than we have time for at the moment.

// func api_gretel_configured_treebanks_with_components(q *Context) {
// 	treebanks := make(map[string]Treebank)

// 	for id := range q.prefixes {
// 		treebanks[id] = Treebank{
// 			// TODO list .dact files as components. retrieve number of sentences and words from the file if possible?
// 			// might be a simple query we can do to get this.
// 			Components:  make(map[string]TreebankComponent),
// 			Description: q.desc[id],
// 			Title:       id,
// 			Metadata:    make([]TreebankMetadata, 0),
// 		}

// 		// extract components (if possible, we use the 'source' metadata field for this, and just count occurances to figure out the size of the subcorpora)
// 		rows, errval := q.db.Query(fmt.Sprintf(
// 			`SELECT text, n
// 			FROM
// 				%s_c_%s_mval as mv
// 					LEFT JOIN
// 				%s_c_%s_midx as mid
// 					ON mv.id = mid.id
// 			WHERE
// 				mid.type = 'text' AND mid.name = 'source'`,
// 			Cfg.Prefix,
// 			id,
// 			Cfg.Prefix,
// 			id))

// 		knownSentences := 0
// 		if !logerr(errval) { // metadata tables exist
// 			for rows.Next() {
// 				var source string
// 				var sentenceCount int
// 				errval = rows.Scan(&source, &sentenceCount)
// 				if logerr(errval) {
// 					rows.Close()
// 					break
// 				}

// 				knownSentences += sentenceCount
// 				treebanks[id].Components[source] = TreebankComponent{
// 					ID:          source,
// 					Description: "",
// 					Sentences:   sentenceCount,
// 					Title:       source,
// 					// can't retrieve from sql db it seems?
// 					// well, we could, I think, but it would require some smart decoding and summing of the mark column in the _deprel table
// 					Words: "?",
// 				}
// 			}
// 		}

// 		if q.lines[id]-knownSentences > 0 {
// 			treebanks[id].Components["unknown"] = TreebankComponent{
// 				ID:          "unknown",
// 				Description: "Sentences of unknown origin",
// 				Sentences:   q.lines[id] - knownSentences, // the rest?
// 				Title:       "unknown",
// 				Words:       "?",
// 			}
// 		}
// 	}

// 	// TODO error handling
// 	rbyte, errval := json.Marshal(treebanks)
// 	if logerr(errval) {
// 		return
// 	}

// 	q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
// 	q.w.Header().Set("Cache-Control", "no-cache")
// 	q.w.Header().Add("Pragma", "no-cache")

//	fmt.Fprint(q.w, string(json.RawMessage(rbyte)[:]))

// }

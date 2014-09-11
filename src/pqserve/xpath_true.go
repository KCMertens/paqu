// +build !nodbxml

package main

import (
	"github.com/pebbe/dbxml"

	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// TAB: xpath
func xpath(q *Context) {

	prefix := getprefix(q)
	if !q.prefixes[prefix] {
		http.Error(q.w, "Invalid corpus: "+prefix, http.StatusPreconditionFailed)
		return
	}

	// HTML-uitvoer van begin van de pagina
	writeHead(q, "", 2)
	html_xpath_header(q)

	// HTML-uitvoer van het formulier
	// Returnwaarde is true als er een query was gedefinieerd
	has_query := html_xpath_form(q)

	// Als er geen query is gedefinieerd, HTML-uitvoer van korte helptekst, pagina-einde, en exit
	if !has_query {
		html_xpath_uitleg(q)
		html_footer(q)
		return
	}

	var chClose <-chan bool
	if f, ok := q.w.(http.CloseNotifier); ok {
		chClose = f.CloseNotify()
	} else {
		chClose = make(<-chan bool)
	}

	_, err := q.db.Exec(fmt.Sprintf("UPDATE `%s_info` SET `active` = NOW() WHERE `id` = %q", Cfg.Prefix, prefix))
	if doErr(q, err) {
		return
	}

	offset := 0
	o, err := strconv.Atoi(first(q.r, "offset"))
	if err == nil {
		offset = o
	}
	if offset < 0 {
		offset = 0
	}

	fmt.Fprintln(q.w, "<hr>")

	if ff, ok := q.w.(http.Flusher); ok {
		ff.Flush()
	}

	now := time.Now()

	var owner string
	rows, err := q.db.Query(fmt.Sprintf("SELECT `owner` FROM `%s_info` WHERE `id` = %q", Cfg.Prefix, prefix))
	if doErr(q, err) {
		return
	}
	for rows.Next() {
		if doErr(q, rows.Scan(&owner)) {
			rows.Close()
			return
		}
	}
	if doErr(q, rows.Err()) {
		return
	}

	dactfiles := make([]string, 0)
	global := false
	if strings.Contains(owner, "@") {
		dactfiles = append(dactfiles, path.Join(paqudir, "data", prefix, "data.dact"))
	} else {
		global = true
		rows, err := q.db.Query(fmt.Sprintf("SELECT `arch` FROM `%s_c_%s_arch` ORDER BY `id`", Cfg.Prefix, prefix))
		if doErr(q, err) {
			return
		}
		for rows.Next() {
			var s string
			if doErr(q, rows.Scan(&s)) {
				rows.Close()
				return
			}
			if strings.HasSuffix(s, ".dact") {
				dactfiles = append(dactfiles, s)
			}
		}
		if doErr(q, rows.Err()) {
			return
		}
	}

	if len(dactfiles) == 0 {
		fmt.Fprintln(q.w, "Er zijn geen dact-bestanden voor dit corpus")
		return
	}

	fmt.Fprintf(q.w, "<ol start=\"%d\" class=\"xpath\">\n", offset+1)

	curno := 0
	filename := ""
	curdac := ""
	xmlall := ""
	xmlparts := make([]string, 0)
	query := first(q.r, "xpath")
	for _, dactfile := range dactfiles {
		select {
		case <-chClose:
			logerr(errConnectionClosed)
			return
		default:
		}
		db, err := dbxml.Open(dactfile)
		if doErr(q, err) {
			return
		}
		docs, err := db.Query(query)
		if err != nil {
			fmt.Fprintln(q.w, "</ol>\n"+html.EscapeString(err.Error()))
			return
		}
		for docs.Next() {
			select {
			case <-chClose:
				docs.Close()
				db.Close()
				logerr(errConnectionClosed)
				return
			default:
			}
			name := docs.Name()
			if name != filename {
				if curno > offset && curno <= offset+ZINMAX*2 {
					xpath_result(q, curno, curdac, filename, xmlall, xmlparts, prefix, global)
					xmlparts = xmlparts[0:0]
				}
				curno++
				curdac = dactfile
				filename = name
			}
			if curno > offset+ZINMAX*2 {
				docs.Close()
			} else {
				if curno > offset && curno <= offset+ZINMAX*2 {
					xmlall = docs.Content()
					xmlparts = append(xmlparts, docs.Match())
				}
			}
		}
		db.Close()
		if curno > offset+ZINMAX*2 {
			break
		}
	}
	if curno > offset && curno <= offset+ZINMAX*2 {
		xpath_result(q, curno, curdac, filename, xmlall, xmlparts, prefix, global)
	}

	fmt.Fprintln(q.w, "</ol>")

	if curno == 0 {
		fmt.Fprintf(q.w, "geen match gevonden")
	}

	// Links naar volgende en vorige pagina's met resultaten
	qs := "xpath=" + urlencode(query)
	if offset > 0 || curno > offset+ZINMAX*2 {
		if offset > 0 {
			fmt.Fprintf(q.w, "<a href=\"/xpath?%s&amp;offset=%d\">vorige</a>", qs, offset-ZINMAX*2)
		} else {
			fmt.Fprint(q.w, "vorige")
		}
		fmt.Fprint(q.w, " | ")
		if curno > offset+ZINMAX*2 {
			fmt.Fprintf(q.w, "<a href=\"/xpath?%s&amp;offset=%d\">volgende</a>", qs, offset+ZINMAX*2)
		} else {
			fmt.Fprint(q.w, "volgende")
		}
	}

	fmt.Fprintln(q.w, "<hr><small>tijd:", time.Now().Sub(now), "</small>")

	html_footer(q)

}

//. HTML

func html_xpath_header(q *Context) {
	fmt.Fprint(q.w, `
<script type="text/javascript"><!--
  function formclear(f) {
    f.xpath.value = "";
  }
  //--></script>
`)
}

func html_xpath_uitleg(q *Context) {
	fmt.Fprint(q.w, `
<p>
<hr>
<p>
Uitleg over XPATH
`)
}

func html_xpath_form(q *Context) (has_query bool) {
	has_query = true
	if first(q.r, "xpath") == "" {
		has_query = false
	}

	fmt.Fprint(q.w, `
<form action="xpath" method="get" accept-charset="utf-8">
corpus: <select name="db">
`)
	html_opts(q, q.opt_db, getprefix(q), "corpus")
	fmt.Fprintln(q.w, "</select>")
	if q.auth {
		fmt.Fprintln(q.w, "<a href=\"corpuslijst\">meer/minder</a>")
	}
	fmt.Fprintf(q.w, `<p>
		XPATH query:<br>
		<textarea name="xpath" rows="6" cols="80">%s</textarea>
		`, html.EscapeString(first(q.r, "xpath")))
	fmt.Fprint(q.w, `<p>
		   <input type="submit" value="Zoeken">
		   <input type="button" value="Wissen" onClick="javascript:formclear(form)">
		   <input type="reset" value="Reset">
	   </form>
	   `)

	return
}

func xpath_result(q *Context, curno int, dactfile, filename, xmlall string, xmlparts []string, prefix string, global bool) {
	alpino := Alpino_ds{}
	err := xml.Unmarshal([]byte(xmlall), &alpino)
	if err != nil {
		fmt.Fprintf(q.w, "<li>FOUT bij parsen van XML: %s\n", html.EscapeString(err.Error()))
		return
	}
	woorden := strings.Fields(alpino.Sentence)

	lvl := make([]int, len(woorden)+1)
	ids := make([]string, len(xmlparts))

	for i, part := range xmlparts {
		alp := Alpino_ds{}
		err := xml.Unmarshal([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<alpino_ds version="1.3">
`+part+`
</alpino_ds>`), &alp)
		if err != nil {
			fmt.Fprintf(q.w, "<li>FOUT bij parsen van XML: %s\n", html.EscapeString(err.Error()))
			return
		}
		ids[i] = alp.Node0.Id
		lvl1 := make([]int, len(woorden)+1)
		alpscan(alp.Node0, alpino.Node0, lvl1)
		for j, n := range lvl1 {
			lvl[j] += n
		}
	}

	fmt.Fprint(q.w, "<li>")
	l := 0
	for i, woord := range woorden {
		for l < lvl[i] {
			l++
			fmt.Fprintf(q.w, "<span class=\"c%d\">", l)
		}
		fmt.Fprintf(q.w, html.EscapeString(woord))
		for l > lvl[i+1] {
			l--
			fmt.Fprint(q.w, "</span>")
		}
		fmt.Fprint(q.w, " ")
	}

	fmt.Fprintf(q.w, "\n<a href=\"/tree?db=%s&amp;names=true&mwu=false&amp;arch=%s&amp;file=%s&amp;global=%v&amp;marknodes=%s\">&nbsp;&#9741;&nbsp;</a>\n",
		prefix,
		html.EscapeString(dactfile),
		html.EscapeString(filename),
		global,
		strings.Join(ids, ","))
}

// zet de teller voor alle woorden onder node 1 hoger
func alpscan(node, node0 *Node, lvl1 []int) {
	if node == nil {
		return
	}
	if strings.TrimSpace(node.Word) != "" {
		lvl1[node.Begin] = 1
	}
	if idx, err := strconv.Atoi(node.Index); err == nil && strings.TrimSpace(node.Word) == "" && len(node.NodeList) == 0 {
		alpscan(alpindex(idx, node0), node0, lvl1)
	}
	for _, n := range node.NodeList {
		alpscan(n, node0, lvl1)
	}
}

// vind de node met een index
func alpindex(idx int, node *Node) *Node {
	if i, err := strconv.Atoi(node.Index); err == nil && i == idx && (strings.TrimSpace(node.Word) != "" || len(node.NodeList) > 1) {
		return node
	}
	for _, n := range node.NodeList {
		if n2 := alpindex(idx, n); n2 != nil {
			return n2
		}
	}
	return nil
}
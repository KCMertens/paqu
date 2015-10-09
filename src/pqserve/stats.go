package main

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"
)

type StatLine struct {
	cols []StructIS
	used bool
}

type StatSorter struct {
	lines  []StatLine
	labels []string
	isInt  []bool
	n      int
}

func (s *StatSorter) Len() int {
	return len(s.lines)
}

func (s *StatSorter) Swap(i, j int) {
	s.lines[i], s.lines[j] = s.lines[j], s.lines[i]
}

func (s *StatSorter) Less(i, j int) bool {
	n := s.n
	if n == 0 {
		if s.lines[i].cols[0].i > s.lines[j].cols[0].i {
			return true
		}
		if s.lines[i].cols[0].i < s.lines[j].cols[0].i {
			return false
		}
	} else if s.isInt[n] {
		if s.lines[i].cols[n].i < s.lines[j].cols[n].i {
			return true
		}
		if s.lines[i].cols[n].i > s.lines[j].cols[n].i {
			return false
		}
	} else {
		// lege strings achteraan
		if s.lines[i].cols[n].s == "" && s.lines[j].cols[n].s != "" {
			return false
		}
		if s.lines[i].cols[n].s != "" && s.lines[j].cols[n].s == "" {
			return true
		}
		if s.lines[i].cols[n].s < s.lines[j].cols[n].s {
			return true
		}
		if s.lines[i].cols[n].s > s.lines[j].cols[n].s {
			return false
		}
	}
	for ii, isint := range s.isInt {
		if ii == n {
			continue
		}
		if ii == 0 {
			if s.lines[i].cols[0].i > s.lines[j].cols[0].i {
				return true
			}
			if s.lines[j].cols[0].i < s.lines[j].cols[0].i {
				return false
			}
		} else if isint {
			if s.lines[i].cols[ii].i < s.lines[j].cols[ii].i {
				return true
			}
			if s.lines[i].cols[ii].i > s.lines[j].cols[ii].i {
				return false
			}
		} else {
			if s.lines[i].cols[ii].s == "" && s.lines[j].cols[ii].s != "" {
				return false
			}
			if s.lines[i].cols[ii].s != "" && s.lines[j].cols[ii].s == "" {
				return true
			}
			if s.lines[i].cols[ii].s < s.lines[j].cols[ii].s {
				return true
			}
			if s.lines[i].cols[ii].s > s.lines[j].cols[ii].s {
				return false
			}
		}
	}
	return false
}

func stats(q *Context) {

	var buf bytes.Buffer

	var chClose <-chan bool
	if f, ok := q.w.(http.CloseNotifier); ok {
		chClose = f.CloseNotify()
	} else {
		chClose = make(<-chan bool)
	}

	now := time.Now()

	download := false
	if first(q.r, "d") != "" {
		download = true
	}

	option := make(map[string]string)
	for _, t := range []string{"word", "postag", "rel", "hpostag", "hword"} {
		option[t] = first(q.r, t)
	}
	if option["word"] == "" && option["postag"] == "" && option["rel"] == "" && option["hpostag"] == "" && option["hword"] == "" {
		http.Error(q.w, "Query ontbreekt", http.StatusPreconditionFailed)
		return
	}

	prefix := first(q.r, "db")
	if prefix == "" {
		http.Error(q.w, "Geen corpus opgegeven", http.StatusPreconditionFailed)
		return
	}
	if !q.prefixes[prefix] {
		http.Error(q.w, "Ongeldig corpus", http.StatusPreconditionFailed)
		return
	}

	query, err := makeQuery(q, prefix, "", chClose)
	if err != nil {
		http.Error(q.w, err.Error(), http.StatusInternalServerError)
		logerr(err)
		return
	}

	// BEGIN UITVOER

	if download {
		q.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		q.w.Header().Set("Content-Disposition", "attachment; filename=telling.txt")
		cache(q)
	} else {
		q.w.Header().Set("Content-Type", "text/html; charset=utf-8")
		cache(q)
		fmt.Fprint(q.w, `<!DOCTYPE html>
<html>
<head>
<title></title>
<script type="text/javascript"><!--
function f(s) {
    window.parent._fn.update(s);
}
//--></script>
</head>
<body">
<script type="text/javascript">
window.parent._fn.started();
</script>
`)
	}

	// DEBUG: HTML-uitvoer van de query
	if !download {
		fmt.Fprint(&buf, "<div style=\"font-family:monospace\">\n", html.EscapeString(query), "\n</div><p>\n")
		updateText(q, buf.String())
		buf.Reset()
	}

	// Aantal zinnen die matchen met de query
	select {
	case <-chClose:
		logerr(errConnectionClosed)
		return
	default:
	}
	rows, err := timeoutQuery(q, chClose, "SELECT DISTINCT `arch`,`file` FROM `"+Cfg.Prefix+"_c_"+prefix+"_deprel` WHERE "+
		query)
	if err != nil {
		updateError(q, err, !download)
		completed(q, download)
		logerr(err)
		return
	}
	counter := 0
	for rows.Next() {
		counter++
	}
	if err != nil {
		updateError(q, err, !download)
		completed(q, download)
		logerr(err)
		return
	}

	if download {
		fmt.Fprintf(q.w, "# %d zinnen\t\n", counter)
	} else {
		fmt.Fprintln(&buf, "Aantal gevonden zinnen:", iformat(counter))
		updateText(q, buf.String())
		buf.Reset()
	}

	// Tellingen van onderdelen
	for i, ww := range []string{"word", "lemma", "postag", "rel", "hword", "hlemma", "hpostag"} {
		var j, count int
		var s, p, limit string
		if download {
			fmt.Fprintln(q.w, "# "+ww+"\t")
		} else {
			if i == 0 {
				fmt.Fprintln(&buf, "<p>"+YELLOW+"<b>word</b></span>: ")
			} else if i == 4 {
				fmt.Fprintln(&buf, "<p>"+GREEN+"<b>hword</b></span>: ")
			} else {
				fmt.Fprintln(&buf, "<p><b>"+ww+"</b>: ")
			}
			limit = " LIMIT " + fmt.Sprint(WRDMAX)
		}
		select {
		case <-chClose:
			logerr(errConnectionClosed)
			return
		default:
		}
		rows, err := timeoutQuery(q, chClose, "SELECT count(*), `"+ww+"` FROM `"+Cfg.Prefix+"_c_"+prefix+
			"_deprel` WHERE "+query+" GROUP BY `"+ww+"` COLLATE 'utf8_bin' ORDER BY 1 DESC, 2"+limit)
		if err != nil {
			updateError(q, err, !download)
			completed(q, download)
			logerr(err)
			return
		}
		for rows.Next() {
			err := rows.Scan(&j, &s)
			if err != nil {
				updateError(q, err, !download)
				completed(q, download)
				logerr(err)
				return
			}
			if s == "" {
				s = "\"\""
			}
			s = unHigh(s)
			if download {
				fmt.Fprintf(q.w, "%d\t%s\n", j, s)
			} else {
				fmt.Fprint(&buf, p, iformat(j), "&times;&nbsp;", html.EscapeString(s))
				p = ", "
				count++
			}
		}
		err = rows.Err()
		if err != nil {
			updateError(q, err, !download)
			completed(q, download)
			logerr(err)
			return
		}
		if !download {
			if count == WRDMAX {
				fmt.Fprint(&buf, ", ...")
			}
			fmt.Fprint(&buf, "\n<BR>\n")
			updateText(q, buf.String())
			buf.Reset()
		}
	}

	if !download {
		fmt.Fprintf(&buf,
			"<hr>tijd: %s\n<p>\n<a href=\"stats?%s&amp;d=1\">download</a>\n",
			tijd(time.Now().Sub(now)),
			strings.Replace(q.r.URL.RawQuery, "&", "&amp;", -1))
		updateText(q, buf.String())
		completed(q, download)
	}
}

func completed(q *Context, download bool) {
	if download {
		return
	}
	fmt.Fprintf(q.w, `<script type="text/javascript">
window.parent._fn.completed();
</script>
`)
}

func statsrel(q *Context) {

	var chClose <-chan bool
	if f, ok := q.w.(http.CloseNotifier); ok {
		chClose = f.CloseNotify()
	} else {
		chClose = make(<-chan bool)
	}

	now := time.Now()

	download := false
	if first(q.r, "d") != "" {
		download = true
	}

	option := make(map[string]string)
	for _, t := range []string{"word", "postag", "rel", "hpostag", "hword"} {
		option[t] = first(q.r, t)
	}
	if option["word"] == "" && option["postag"] == "" && option["rel"] == "" && option["hpostag"] == "" && option["hword"] == "" {
		http.Error(q.w, "Query ontbreekt", http.StatusPreconditionFailed)
		return
	}

	prefix := first(q.r, "db")
	if prefix == "" {
		http.Error(q.w, "Geen corpus opgegeven", http.StatusPreconditionFailed)
		return
	}
	if !q.prefixes[prefix] {
		http.Error(q.w, "Ongeldig corpus", http.StatusPreconditionFailed)
		return
	}

	var metas []MetaType
	if q.hasmeta[prefix] {
		metas = getMeta(q, prefix)
	}
	metai := make(map[string]int)
	metat := make(map[string]string)
	for _, meta := range metas {
		metai[meta.name] = meta.id
		metat[meta.name] = meta.mtype
	}

	qselect := make([]string, 0)
	qfrom := fmt.Sprintf("`%s_c_%s_deprel` `a`", Cfg.Prefix, prefix)
	qwhere := ""
	qgo := make([]string, 0)
	ncols := 0
	fields := make([]interface{}, 0)
	fields = append(fields, new(int))

	cols := make([]string, 1, 8)
	aligns := make([]string, 1, 8)
	aligns[0] = "right"
	for _, t := range []string{"word", "lemma", "postag", "rel", "hword", "hlemma", "hpostag"} {
		if first(q.r, "c"+t) == "1" {
			f := "`a`.`" + t + "`"
			qselect = append(qselect, f)
			qgo = append(qgo, f)
			ncols++
			cols = append(cols, t)
			aligns = append(aligns, "left")
			fields = append(fields, new(string))
		}
	}
	nattr := ncols

	for _, meta := range q.r.Form["cmeta"] {
		cols = append(cols, "meta:"+meta, "idx:"+meta)
		if metat[meta] == "TEXT" {
			aligns = append(aligns, "left")
		} else {
			aligns = append(aligns, "right")
		}
		ncols += 2
		fields = append(fields, new(string), new(int))
		table := fmt.Sprintf("m%d", metai[meta])
		qfrom += fmt.Sprintf(" JOIN ( `%s_c_%s_meta` `%s`", Cfg.Prefix, prefix, table)
		qfrom += fmt.Sprintf(" JOIN `%s_c_%s_mval` `%sv` USING(`id`,`idx`)", Cfg.Prefix, prefix, table)
		qfrom += " ) USING(`arch`,`file`)"
		qwhere += fmt.Sprintf(" AND `%s`.`id` = %d", table, metai[meta])
		f := "`" + table + "v`.`text`"
		fi := "`" + table + "v`.`idx`"
		qselect = append(qselect, f, fi)
		qgo = append(qgo, fi)
	}

	query, err := makeQuery(q, prefix, "a", chClose)
	if err != nil {
		http.Error(q.w, err.Error(), http.StatusInternalServerError)
		logerr(err)
		return
	}

	qgos := strings.Join(qgo, ",")
	fullquery := fmt.Sprintf("SELECT count(*),%s FROM %s WHERE %s %s GROUP BY %s ORDER BY 1 DESC, %s",
		strings.Join(qselect, ","), qfrom, query, qwhere, qgos, qgos)

	// BEGIN UITVOER

	if download {
		q.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		q.w.Header().Set("Content-Disposition", "attachment; filename=telling.txt")
	} else {
		q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	cache(q)

	select {
	case <-chClose:
		logerr(errConnectionClosed)
		return
	default:
	}

	if !download {
		fullquery += fmt.Sprintf(" LIMIT %d", BIGLIMIT)
	}

	rows, err := timeoutQuery(q, chClose, fullquery)
	if err != nil {
		if download {
			interneFoutRegel(q, err, !download)
		} else {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
		}
		logerr(err)
		return
	}

	if download {
		fmt.Fprint(q.w, "aantal")
		for _, c := range cols[1:] {
			if !strings.HasPrefix(c, "idx:") {
				fmt.Fprint(q.w, "\t"+c)
			}
		}
		fmt.Fprintln(q.w)
	}

	n := 0
	var data *StatSorter
	if !download {
		data = &StatSorter{
			lines:  make([]StatLine, 0),
			labels: make([]string, 1),
			isInt:  make([]bool, 1),
		}
		data.labels[0] = "aantal"
		data.isInt[0] = true
		for _, col := range cols[1:] {
			if strings.HasPrefix(col, "idx:") {
				continue
			}
			if strings.HasPrefix(col, "meta:") {
				data.labels = append(data.labels, html.EscapeString(col[5:]))
				data.isInt = append(data.isInt, true)
			} else {
				data.labels = append(data.labels, col)
				data.isInt = append(data.isInt, false)
			}
		}
	}

	for rows.Next() {
		n++
		err := rows.Scan(fields...)
		if err != nil {
			if download {
				interneFoutRegel(q, err, !download)
			} else {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
			}
			logerr(err)
			return
		}

		dataline := StatLine{cols: make([]StructIS, 0)}

		var link string
		if !download {
			if nattr > 0 {
				// attributen in kolom 1 t/m kolom nattr
				var qword, qpostag, qrel, qhword, qhpostag string
				for j := nattr; j > 0; j-- { // van achter naar voor zodat word prioriteit krijgt over lemma
					if sp, ok := fields[j].(*string); ok {
						s := *sp
						switch cols[j] {
						case "word":
							qword = urlencode("=" + unHigh(s))
						case "lemma":
							qword = urlencode("+" + unHigh(s))
						case "postag":
							qpostag = urlencode(s)
						case "rel":
							qrel = urlencode(s)
						case "hword":
							qhword = urlencode("=" + unHigh(s))
						case "hlemma":
							qhword = urlencode("+" + unHigh(s))
						case "hpostag":
							qhpostag = urlencode(s)
							if qhpostag == "" {
								qhpostag = "--LEEG--"
							}
						}
					}
				}
				link = fmt.Sprintf(
					"db=%s&amp;word=%s&amp;postag=%s&amp;rel=%s&amp;hword=%s&amp;hpostag=%s",
					urlencode(first(q.r, "db")), qword, qpostag, qrel, qhword, qhpostag)
			}
		}

		for i, e := range fields {
			if strings.HasPrefix(cols[i], "idx:") {
				continue
			}
			var value string
			switch v := e.(type) {
			case *string:
				value = unHigh(*v)
			case *int:
				if download {
					value = fmt.Sprint(*v)
				} else {
					value = fmt.Sprint(iformat(*v))
				}
			}
			if download {
				var t string
				if i != 0 {
					t = "\t"
				}
				fmt.Fprintf(q.w, t+value)
				continue
			}

			if strings.HasPrefix(cols[i], "meta:") {
				dataline.cols = append(dataline.cols, StructIS{*fields[i+1].(*int), value})
			} else {
				var c int
				if i == 0 {
					value = link
					c = *e.(*int)
				}
				dataline.cols = append(dataline.cols, StructIS{c, value})
			}
		}
		if download {
			fmt.Fprintln(q.w)
		} else {
			data.lines = append(data.lines, dataline)
		}
	}

	if download {
		return
	}

	if len(data.lines) > WRDMAX {
		for i := 1; i < len(data.isInt); i++ {
			data.n = i
			sort.Sort(data)
			for j := range data.lines {
				if j == WRDMAX {
					break
				}
				data.lines[j].used = true
			}
		}
		data.n = 0
		sort.Sort(data)
	}

	s := ""
	fmt.Fprintf(q.w, "{\n\"toomany\": %v,\n\"aligns\": [", len(data.lines) == BIGLIMIT)
	for _, a := range aligns {
		fmt.Fprintf(q.w, "%s%q", s, a)
		s = ","
	}
	s = ""
	fmt.Fprint(q.w, "],\n\"labels\": [")
	for _, lbl := range data.labels {
		fmt.Fprintf(q.w, "%s%q", s, lbl)
		s = ","
	}
	s = ""
	fmt.Fprint(q.w, "],\n\"isint\": [")
	for _, a := range data.isInt {
		fmt.Fprintf(q.w, "%s%v", s, a)
		s = ","
	}
	fmt.Fprintln(q.w, "],\n\"lines\": [")
	var s1, s2 string
	for i, line := range data.lines {
		if i >= WRDMAX && !line.used {
			continue
		}
		fmt.Fprintf(q.w, "%s[", s1)
		s1 = ",\n"
		s2 = ""
		for _, col := range line.cols {
			fmt.Fprintf(q.w, "%s[%q,%d]", s2, col.s, col.i)
			s2 = ","
		}
		fmt.Fprint(q.w, "]")
	}
	fmt.Fprintln(q.w, "\n],")

	fmt.Fprintf(q.w, "\"tijd\": %q,\n\"download\": %q,\n\"query\": %q\n}\n",
		tijd(time.Now().Sub(now)),
		strings.Replace(q.r.URL.RawQuery, "&", "&amp;", -1)+"&amp;d=1",
		html.EscapeString(query))
}

func interneFoutRegel(q *Context, err error, is_html bool) {
	s := err.Error()
	if is_html {
		s = html.EscapeString(s)
	}
	fmt.Fprintln(q.w, "Interne fout:", s)
}

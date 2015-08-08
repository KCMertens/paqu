package main

import (
	"archive/zip"
	"compress/gzip"
	"database/sql"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

func savez(q *Context) {

	if !q.auth {
		http.Error(q.w, "Je bent niet ingelogd", http.StatusUnauthorized)
		return
	}

	writeHead(q, "Nieuw corpus maken", 0)

	fmt.Fprint(q.w, `
<script type="text/javascript" src="jquery.js"></script>
<script type="text/javascript"><!--
var submitted = false;
function submitter() {
    if (submitted) {
        return false;
    }
    submitted = true;
    $('#subbut').addClass('hide');
    $('#subsp').removeClass('hide');
    return true;
}
//<input type="submit" value="nieuw corpus maken" id="subbut">
//<span id="subsp" class="hide">Even geduld...</span>
//--></script>
<form action="savez2" onsubmit="javascript:return submitter()">
Kies een of meer corpora:
<p>
`)
	choice := make(map[string]string)
	for _, c := range q.r.Form["db"] {
		choice[c] = " checked"
	}
	var gr byte
	for _, c := range q.opt_db {
		if c[0] != gr {
			gr = c[0]
			var t string
			switch gr {
			case 'A':
				t = "algemene corpora"
			case 'B':
				t = "mijn corpora"
			case 'C':
				t = "corpora gedeeld door anderen"
			}
			fmt.Fprintln(q.w, "<b>&mdash;", t, "&mdash;</b><br>")
		}
		p := strings.Fields(c)
		txt := strings.Join(p[1:], " ")
		opt := p[0][1:]
		fmt.Fprintf(q.w, "<input type=\"checkbox\" name=\"db\" value=\"%s\"%s>%s<br>\n",
			opt,
			choice[opt],
			html.EscapeString(txt))
	}

	fmt.Fprintf(q.w, `
<p>
Zoekopdracht:
<p>
<table>
<tr>
<td style="background-color: yellow">woord
<td>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
<td style="background-color: lightgreen">hoofdwoord
<tr><td><em>%s</em><td>%s<td><em>%s</em>
<tr><td>%s<td><td>%s
</table>
<p>
Titel:<br>
<input type="text" name="title" size="80" maxlength="64">
`,
		html.EscapeString(first(q.r, "word")),
		html.EscapeString(first(q.r, "rel")),
		html.EscapeString(first(q.r, "hword")),
		html.EscapeString(first(q.r, "postag")),
		html.EscapeString(first(q.r, "hpostag")))

	s := "default: geen limit"
	if Cfg.Maxdup > 0 {
		s = fmt.Sprintf("default is maximum van %d", Cfg.Maxdup)
	}
	fmt.Fprintf(q.w, `
<p>
Maximum aantal zinnen (%s):<br>
<input type="text" name="maxdup" size="8" maxlength="10">
`, s)

	fmt.Fprintf(q.w, `<p>
<input type="hidden" name="word" value="%s">
<input type="hidden" name="postag" value="%s">
<input type="hidden" name="rel" value="%s">
<input type="hidden" name="hpostag" value="%s">
<input type="hidden" name="hword" value="%s">
<input type="submit" value="nieuw corpus maken" id="subbut">
<span id="subsp" class="hide">Even geduld...</span>
`,
		html.EscapeString(first(q.r, "word")),
		html.EscapeString(first(q.r, "postag")),
		html.EscapeString(first(q.r, "rel")),
		html.EscapeString(first(q.r, "hpostag")),
		html.EscapeString(first(q.r, "hword")))

	fmt.Fprint(q.w, `
</body>
</html>
`)

}

func savez2(q *Context) {

	var fpz, fpgz *os.File
	var z *zip.Writer
	var gz *gzip.Reader
	var dact interface{}
	var err error

	close := func() {
		if z != nil {
			z.Close()
		}
		if fpz != nil {
			fpz.Close()
		}
		if gz != nil {
			gz.Close()
		}
		if fpgz != nil {
			fpgz.Close()
		}
		saveCloseDact(dact)
	}

	protected := 0

	if !q.auth {
		http.Error(q.w, "Je bent niet ingelogd", http.StatusUnauthorized)
		return
	}

	corpora := q.r.Form["db"]
	for _, corpus := range corpora {
		if !q.prefixes[corpus] {
			http.Error(q.w, "Geen toegang tot corpus", http.StatusUnauthorized)
			return
		}
		if q.protected[corpus] || !q.myprefixes[corpus] {
			protected = 1
		}
	}

	if len(corpora) == 0 {
		writeHtml(q, "Fout", "Geen corpora gekozen")
		return
	}

	word := first(q.r, "word")
	rel := first(q.r, "rel")
	hword := first(q.r, "hword")
	postag := first(q.r, "postag")
	hpostag := first(q.r, "hpostag")
	if word == "" && hword == "" && rel == "" && postag == "" && hpostag == "" {
		writeHtml(q, "Fout", "Zoektermen ontbreken")
		return
	}

	title := maxtitlelen(first(q.r, "title"))
	if title == "" {
		writeHtml(q, "Fout", "Titel ontbreekt")
		return
	}

	maxdup, _ := strconv.Atoi(first(q.r, "maxdup"))
	if maxdup < 1 || maxdup > Cfg.Maxdup {
		maxdup = Cfg.Maxdup
	}

	dirname, fulldirname, ok := beginNewCorpus(q, title)
	if !ok {
		return
	}

	fpz, err = os.Create(fulldirname + "/data")
	if err != nil {
		http.Error(q.w, err.Error(), http.StatusInternalServerError)
		logerr(err)
		return
	}
	z = zip.NewWriter(fpz)

	linecount := 0

	chClose := make(<-chan bool)
	for _, prefix := range corpora {
		if linecount == maxdup && maxdup > 0 {
			break
		}

		global, ok := isGlobal(q, prefix)
		if !ok {
			close()
			return
		}
		pathlen, ok := getPathLen(q, prefix, global)
		if !ok {
			close()
			return
		}

		query, err := makeQuery(q, prefix, chClose)
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			close()
			return
		}
		query = strings.Replace(query, " `", " `c`.`", -1)
		query = fmt.Sprintf("SELECT DISTINCT `f`.`file`, `c`.`arch` FROM `%s_c_%s_deprel` `c`, `%s_c_%s_file` `f` WHERE `c`.%s AND `f`.`id` = `c`.`file`",
			Cfg.Prefix, prefix,
			Cfg.Prefix, prefix,
			query)
		rows, err := q.db.Query(query)
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			close()
			return
		}
		currentarch := -1
		dact = nil
		var arch int
		var filename, dactname string
		for rows.Next() {
			if linecount == maxdup && maxdup > 0 {
				rows.Close()
				break
			}
			err := rows.Scan(&filename, &arch)
			if err != nil {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				close()
				return
			}

			var data []byte
			if arch < 0 {
				fpgz, err = os.Open(filename + ".gz")
				if err == nil {
					gz, err = gzip.NewReader(fpgz)
					if err != nil {
						http.Error(q.w, err.Error(), http.StatusInternalServerError)
						logerr(err)
						close()
						return
					}
					data, err = ioutil.ReadAll(gz)
					if err != nil {
						http.Error(q.w, err.Error(), http.StatusInternalServerError)
						logerr(err)
						close()
						return
					}
					gz.Close()
					gz = nil
					fpgz.Close()
					fpgz = nil
				} else {
					fpgz, err = os.Open(filename)
					if err != nil {
						http.Error(q.w, err.Error(), http.StatusInternalServerError)
						logerr(err)
						close()
						return
					}
					data, err = ioutil.ReadAll(fpgz)
					if err != nil {
						http.Error(q.w, err.Error(), http.StatusInternalServerError)
						logerr(err)
						close()
						return
					}
					fpgz.Close()
					fpgz = nil
				}

			} else {

				if arch != currentarch {
					currentarch = arch
					saveCloseDact(dact)
					dact, dactname = saveOpenDact(q, prefix, arch)
				}

				data = saveGetDact(q, dact, filename)

			}

			var newfile string
			if arch < 0 {
				newfile = filename[pathlen:]
				if !global {
					if strings.Contains(q.params[prefix], "-lbl") || q.params[prefix] == "folia" || q.params[prefix] == "tei" {
						newfile = decode_filename(newfile[10:])
					} else if strings.HasPrefix(q.params[prefix], "xmlzip") || q.params[prefix] == "dact" {
						newfile = decode_filename(newfile[5:])
					}
				}
			} else {
				newfile = dactname[pathlen:] + "::" + filename
			}
			if len(corpora) > 1 {
				newfile = prefix + "/" + newfile
			}

			f, err := z.Create(newfile)
			if err != nil {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				close()
				return
			}
			_, err = f.Write(data)
			if err != nil {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				close()
				return
			}
			linecount++
		}
		err = rows.Err()
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			close()
			return
		}
		saveCloseDact(dact)
		dact = nil
	}

	err = z.Close()
	if err != nil {
		http.Error(q.w, err.Error(), http.StatusInternalServerError)
		logerr(err)
		z = nil
		close()
		return
	}
	fpz.Close()

	s := "xmlzip-d"
	if protected != 0 {
		s = "xmlzip-p"
	}
	newCorpus(q, dirname, title, s, protected)
}

func isGlobal(q *Context, prefix string) (global bool, ok bool) {
	rows, err := q.db.Query(fmt.Sprintf("SELECT `owner` FROM `%s_info` WHERE `id` = %q", Cfg.Prefix, prefix))
	if err != nil {
		http.Error(q.w, err.Error(), http.StatusInternalServerError)
		logerr(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		err = rows.Scan(&s)
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			return
		}
		global = !strings.Contains(s, "@")
	}
	return global, true
}

func getPathLen(q *Context, prefix string, global bool) (length int, ok bool) {

	if !global {
		return len(path.Join(paqudir, "data", prefix, "xml")) + 1, true
	}

	var min, max sql.NullInt64
	rows, err := q.db.Query(fmt.Sprintf("SELECT min(`file`), max(`file`) FROM `%s_c_%s_sent` WHERE `arch` = -1", Cfg.Prefix, prefix))
	if err != nil {
		http.Error(q.w, err.Error(), http.StatusInternalServerError)
		logerr(err)
		return
	}
	for rows.Next() {
		err := rows.Scan(&min, &max)
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			rows.Close()
			return
		}
	}
	rows.Close()

	files := make([]string, 0, 2)
	if min.Valid && max.Valid {
		rows, err = q.db.Query(fmt.Sprintf("SELECT `file` FROM `%s_c_%s_file` WHERE `id` = %d OR `id` = %d",
			Cfg.Prefix, prefix, min.Int64, max.Int64))
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			return
		}
		for rows.Next() {
			var s string
			err := rows.Scan(&s)
			if err != nil {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				rows.Close()
				return
			}
			files = append(files, s)
		}
		rows.Close()
	} else {
		rows, err := q.db.Query(fmt.Sprintf("SELECT min(`id`), max(`id`) FROM `%s_c_%s_arch`", Cfg.Prefix, prefix))
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			return
		}
		for rows.Next() {
			err := rows.Scan(&min, &max)
			if err != nil {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				rows.Close()
				return
			}
		}
		rows.Close()
		rows, err = q.db.Query(fmt.Sprintf("SELECT `arch` FROM `%s_c_%s_arch` WHERE `id` = %d OR `id` = %d",
			Cfg.Prefix, prefix, min.Int64, max.Int64))
		if err != nil {
			http.Error(q.w, err.Error(), http.StatusInternalServerError)
			logerr(err)
			return
		}
		for rows.Next() {
			var s string
			err := rows.Scan(&s)
			if err != nil {
				http.Error(q.w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				rows.Close()
				return
			}
			files = append(files, s)
		}
		rows.Close()
	}
	if len(files) == 1 {
		files = append(files, files[0])
	}
	if len(files) != 2 {
		return
	}

	a1 := strings.Split(files[0], "/")
	a2 := strings.Split(files[1], "/")
	var i int
	for i = 0; i < len(a1)-1 && i < len(a2)-1; i++ {
		if a1[i] != a2[i] {
			break
		}
	}
	return len(strings.Join(a1[:i], "/")) + 1, true
}

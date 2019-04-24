package main

import (
	"github.com/dchest/authcookie"

	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
)

// The context acts as global store for a single request

type Context struct {
	w            http.ResponseWriter
	r            *http.Request
	user         string
	auth         bool
	sec          string
	quotum       int
	db           *sql.DB
	opt_db       []string
	opt_dbmeta   []string
	opt_dbspod   []string
	ignore       map[string]bool
	prefixes     map[string]bool
	spodprefixes map[string]bool
	myprefixes   map[string]bool
	protected    map[string]bool
	hasmeta      map[string]bool
	desc         map[string]string
	lines        map[string]int
	words        map[string]int
	shared       map[string]string
	params       map[string]string
	form         *multipart.Form
}

// Wrap handler in minimale context, net genoeg voor afhandelen statische pagina's
func handleStatic(url string, handler func(*Context)) {
	url = path.Join("/", url)
	http.HandleFunc(
		url,
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != url {
				http.NotFound(w, r)
				return
			}
			q := &Context{
				w: w,
				r: r,
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			handler(q)
		})
}

// Wrap handler in complete context
func handleFunc(url string, handler func(*Context)) {
	oldURL := url
	url = path.Join("/", url)
	if strings.HasSuffix(oldURL, "/") {
		url += "/"
	}

	http.HandleFunc(
		url,
		func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, url) {
				http.NotFound(w, r)
				return
			}

			q := &Context{
				w:            w,
				r:            r,
				opt_db:       make([]string, 0),
				opt_dbmeta:   make([]string, 0),
				opt_dbspod:   make([]string, 0),
				prefixes:     make(map[string]bool),
				myprefixes:   make(map[string]bool),
				spodprefixes: make(map[string]bool),
				protected:    make(map[string]bool),
				hasmeta:      make(map[string]bool),
				desc:         make(map[string]string),
				lines:        make(map[string]int),
				words:        make(map[string]int),
				shared:       make(map[string]string),
				params:       make(map[string]string),
			}

			// Maak verbinding met database
			var err error
			q.db, err = dbopen()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				return
			}
			defer q.db.Close()

			// Is de gebruiker ingelogd?
			if auth, err := r.Cookie("paqu-auth"); err == nil {
				s := strings.SplitN(authcookie.Login(auth.Value, []byte(getRemote(q)+Cfg.Secret)), "|", 2)
				if len(s) == 2 {
					q.user = s[1]
					q.sec = s[0]
				}
			}
			if q.user != "" {
				rows, err := q.db.Query(fmt.Sprintf(
					"SELECT SQL_CACHE `quotum` FROM `%s_users` WHERE `mail` = %q AND `sec` = %q", Cfg.Prefix, q.user, q.sec))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					logerr(err)
					return
				}
				if !rows.Next() {
					q.user = ""
				} else {
					err := rows.Scan(&q.quotum)
					rows.Close()
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						logerr(err)
						return
					}
					q.auth = true
					_, err = q.db.Exec(fmt.Sprintf("UPDATE `%s_users` SET `active` = NOW() WHERE `mail` = %q", Cfg.Prefix, q.user))
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						logerr(err)
						return
					}
				}
			}

			// Laad lijsten van corpora

			q.ignore = make(map[string]bool)
			if q.auth {
				rows, err := q.db.Query(fmt.Sprintf("SELECT `prefix` FROM `%s_ignore` WHERE `user` = %q", Cfg.Prefix, q.user))
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
						return
					}
					q.ignore[s] = true
				}
			}

			s := "IF(`i`.`owner` = \"none\", \"C\", IF(`i`.`owner` = \"auto\", \"B\",  IF(`i`.`owner` = \"manual\", \"A\", \"Z\")))"
			where := ""
			if q.auth {
				s = fmt.Sprintf("IF(`i`.`owner` = \"none\", \"C\", IF(`i`.`owner` = \"auto\", \"B\", IF(`i`.`owner` = \"manual\", \"A\", IF(`i`.`owner` = %q, \"D\", \"E\"))))", q.user)
				where = fmt.Sprintf(" OR `c`.`user` = %q", q.user)
			}
			rows, err := q.db.Query(fmt.Sprintf(
				"SELECT SQL_CACHE `i`.`id`, `i`.`description`, `i`.`nline`, `i`.`nword`, `i`.`owner`, `i`.`shared`, `i`.`params`,  "+s+", `i`.`protected`, `i`.`hasmeta` "+
					"FROM `%s_info` `i`, `%s_corpora` `c` "+
					"WHERE `c`.`enabled` = 1 AND "+
					"`i`.`status` = \"FINISHED\" AND `i`.`id` = `c`.`prefix` AND ( `c`.`user` = \"all\"%s ) "+
					"ORDER BY 7, 2",
				Cfg.Prefix,
				Cfg.Prefix,
				where))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				logerr(err)
				return
			}
			var id, desc, owner, shared, params, group string
			var zinnen, woorden, protected, hasmeta int
			for rows.Next() {
				err := rows.Scan(&id, &desc, &zinnen, &woorden, &owner, &shared, &params, &group, &protected, &hasmeta)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					logerr(err)
					return
				}
				if group == "E" {
					if !q.ignore[id] {
						q.opt_db = append(q.opt_db, fmt.Sprintf("E%s %s \u2014 %s \u2014 %s zinnen", id, desc, displayEmail(owner), iformat(zinnen)))
						q.prefixes[id] = true
						if hasmeta > 0 {
							q.opt_dbmeta = append(q.opt_dbmeta, fmt.Sprintf("E%s %s \u2014 %s \u2014 %s zinnen",
								id, desc, displayEmail(owner), iformat(zinnen)))
						}
						if Cfg.Maxspodlines < 1 || zinnen <= Cfg.Maxspodlines {
							q.opt_dbspod = append(q.opt_dbspod, fmt.Sprintf("E%s %s \u2014 %s \u2014 %s zinnen",
								id, desc, displayEmail(owner), iformat(zinnen)))
							q.spodprefixes[id] = true
						}
					}
				} else if q.auth || owner == "none" || owner == "auto" || owner == "manual" {
					q.opt_db = append(q.opt_db, fmt.Sprintf("%s%s %s \u2014 %s zinnen", group, id, desc, iformat(zinnen)))
					q.prefixes[id] = true
					if hasmeta > 0 {
						q.opt_dbmeta = append(q.opt_dbmeta, fmt.Sprintf("%s%s %s \u2014 %s zinnen", group, id, desc, iformat(zinnen)))
					}
					if Cfg.Maxspodlines < 1 || zinnen <= Cfg.Maxspodlines {
						q.opt_dbspod = append(q.opt_dbspod, fmt.Sprintf("%s%s %s \u2014 %s zinnen", group, id, desc, iformat(zinnen)))
						q.spodprefixes[id] = true
					}
				}
				q.desc[id] = desc
				q.lines[id] = zinnen
				q.words[id] = woorden
				q.shared[id] = shared
				q.params[id] = params
				q.protected[id] = protected > 0
				if hasmeta > 0 {
					q.hasmeta[id] = true
				}
				if q.auth && owner == q.user {
					q.myprefixes[id] = true
				}
			}

			// Verwerk input
			switch r.Method {
			case "OPTIONS":
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Accept-Language, Content-Language, Content-Type")
				return
			case "GET":
				err = r.ParseForm()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					logerr(err)
					return
				}
			case "POST":
				if url != "/corsave" {
					reader, err := r.MultipartReader()
					if err == nil {
						q.form, err = reader.ReadForm(10 * 1024 * 1024)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							logerr(err)
							return
						}
					}
					// let the implementation site figure it out
				}
			default:
				http.Error(w, "Methode "+r.Method+" is niet toegestaan", http.StatusMethodNotAllowed)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", "*")

			// Update login-cookies
			setcookie(q)

			handler(q)
		})
}

// Laat niet meer dan een deel van een e-mailadres zien
func displayEmail(s string) string {
	p1 := strings.Index(s, "@")
	p2 := strings.LastIndex(s, ".")
	if p1 < 0 || p2 < 0 {
		return s
	}
	return s[0:p1+1] + ".." + s[p2:len(s)]
}

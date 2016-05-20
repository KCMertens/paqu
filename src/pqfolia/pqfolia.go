package main

import (
	"github.com/BurntSushi/toml"
	"github.com/beevik/etree"
	"github.com/pebbe/util"

	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Meta_dir   string
	Data_dir   string
	Output_dir string
	Meta_src   string
	File_src   string
	File_path  string
	Tokenized  bool
	Items      map[string]Item
	Item_list  []string
}

type Item struct {
	Type  string
	XPath string
	ID    string
}

type Native struct {
	Label string
	Type  string
}

type State struct {
	speaker    string
	inMetadata bool
	inMeta     bool
	inS        bool
	inW        bool
	inT        bool
	inSkip     bool
}

var (
	x            = util.CheckErr
	cfg          Config
	Map          = make(map[string]string)
	fixed        = make([]string, 0)
	nonfixed     = make([]string, 0)
	native       = make([]string, 0)
	native_use   = make(map[string]bool)
	native_items = make(map[string]Native)
	native_seen  map[string]bool
	currentfile  string
	pathlevel    = 0
	fpout        = os.Stdout
	fileno       = 0

	opt_n = flag.Int("n", 0, "maximum aantal bestanden")
	opt_m = flag.Int("m", 0, "maximum aantal zinner per bestand")
)

func usage() {
	fmt.Fprintf(os.Stderr, `
Syntax: %s [-n int] [-m int] configfile.toml

  -n: maximum aantal bestanden (voor testen)
  -m: maximum aantal zinner per bestand (voor testen)

`, os.Args[0])
}

func main() {

	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		return
	}

	cfg.Tokenized = true
	cfg.Meta_src = "Meta.Src"
	cfg.File_src = "File.Src"
	cfg.File_path = "File.Path."
	md, err := toml.DecodeFile(flag.Arg(0), &cfg)
	x(err)
	if un := md.Undecoded(); len(un) != 0 {
		fmt.Fprintln(os.Stderr, "De volgende items in", flag.Arg(0), "zijn onbekend. Spelfout?")
		for _, u := range un {
			fmt.Fprintln(os.Stderr, "  ->", u)
		}
		return
	}

	for _, i := range cfg.Item_list {
		it, ok := cfg.Items[i]
		if !ok {
			fmt.Fprintln(os.Stderr, "Geen definitie gevonden voor:", i)
			return
		}
		if it.ID != "" {
			native = append(native, it.ID)
			native_use[it.ID] = true
			native_items[it.ID] = Native{
				Label: i,
				Type:  it.Type,
			}
		} else if strings.Contains(it.XPath, "%speaker%") {
			nonfixed = append(nonfixed, i)
		} else {
			fixed = append(fixed, i)
		}
	}

	doDir("")
	if cfg.Output_dir == "" {
		doEnd()
		native_seen = make(map[string]bool)
		doFixed(false, false, false)
	}
}

func doDir(p string) {

	if *opt_n > 0 && *opt_n == fileno {
		return
	}

	dir := filepath.Join(cfg.Data_dir, p)
	fmt.Fprintln(os.Stderr, ">>>", dir)
	files, err := ioutil.ReadDir(dir)
	x(err)
	if cfg.Output_dir != "" {
		x(os.MkdirAll(filepath.Join(cfg.Output_dir, p), 0755))
	}
	for _, file := range files {
		if *opt_n > 0 && *opt_n == fileno {
			return
		}
		if file.IsDir() {
			doDir(filepath.Join(p, file.Name()))
		} else {
			doFile(file.Name(), p)
		}
	}
}

func doFile(filename, dirname string) {
	if !strings.HasSuffix(filename, ".xml") {
		return
	}

	fileno++
	lineno := 0

	native_seen = make(map[string]bool)

	if cfg.Output_dir != "" {
		var f string
		if strings.HasSuffix(filename, ".xml") {
			f = filename[:len(filename)-4] + ".txt"
		} else {
			f = filename + ".txt"
		}
		var err error
		fpout, err = os.Create(filepath.Join(cfg.Output_dir, dirname, f))
		x(err)
		defer fpout.Close()
		pathlevel = 0
	}

	if cfg.File_src != "" {
		fmt.Fprintf(fpout, "##META text %s = %s\n", cfg.File_src, filename)
	}

	if cfg.File_path != "" {
		if dirname == "" {
			for i := 0; i < pathlevel; i++ {
				fmt.Fprintf(fpout, "##META text %s%d =\n", cfg.File_path, i+1)
			}
			pathlevel = 0
		} else {
			parts := strings.Split(dirname, string(os.PathSeparator))
			for i, p := range parts {
				fmt.Fprintf(fpout, "##META text %s%d = %s\n", cfg.File_path, i+1, p)
			}
			for i := len(parts); i < pathlevel; i++ {
				fmt.Fprintf(fpout, "##META text %s%d =\n", cfg.File_path, i+1)
			}
			pathlevel = len(parts)
		}
	}

	filename = filepath.Join(dirname, filename)

	fmt.Fprintln(os.Stderr, ">", filename)

	var doc *etree.Document
	statestack := make([]State, 1, 10)
	currentspeaker := " oiqoewij doijqowiu98793olj fdowqjoiequ8nf  fke f wf  wejfo  fwoiu92  "
	values := make(map[string]map[string][]string)
	hasMeta := false
	fixedDone := false
	nativeDone := false

	currentfile = filepath.Join(cfg.Data_dir, filename)
	fpin, err := os.Open(currentfile)
	x(err)
	defer fpin.Close()
	d := xml.NewDecoder(fpin)
	var meta, label string
	var teller uint64
	words := make([]string, 0, 500)
PARSE:
	for {
		tt, err := d.Token()
		if err == io.EOF {
			break
		}
		x(err)

		if t, ok := tt.(xml.StartElement); ok {

			state := statestack[len(statestack)-1]

			for _, e := range t.Attr {
				switch e.Name.Local {
				case "speaker":
					state.speaker = e.Value
				case "class":
					for _, s := range strings.Fields(e.Value) {
						if s == "original" {
							state.inSkip = true
							break
						}
					}
				}
			}

			switch t.Name.Local {
			case "metadata":
				state.inMetadata = true
				var src string
				for _, e := range t.Attr {
					if e.Name.Local == "src" {
						src = e.Value
						break
					}
				}
				if src != "" {

					if cfg.Meta_src != "" {
						fmt.Fprintf(fpout, "##META text %s = %s\n", cfg.Meta_src, src)
					}

					srcs := make([]string, 0, 4)
					if dirname != "" {
						srcs = append(srcs, filepath.Join(dirname, src+".xml"), filepath.Join(dirname, src))
					}
					srcs = append(srcs, src+".xml", src)

					for i, src := range srcs {
						doc = etree.NewDocument()
						err := doc.ReadFromFile(filepath.Join(cfg.Meta_dir, src))
						if err == nil {
							break
						}
						if i == len(srcs)-1 {
							x(err)
						}
					}

					for _, item := range fixed {
						found := false
						for _, t := range doc.FindElements(cfg.Items[item].XPath) {
							value := t.Text()
							if value != "" && oktype(item, value) {
								found = true
								fmt.Fprintf(fpout, "##META %s %s = %s\n", cfg.Items[item].Type, item, value)
							}
						}
						if !found {
							fmt.Fprintf(os.Stderr, "Niet gevonden in %s voor (%s) %s\n", currentfile, cfg.Items[item].Type, item)
							fmt.Fprintf(fpout, "##META %s %s =\n", cfg.Items[item].Type, item)
						}
					}

					hasMeta = true
					fixedDone = true
				}
			case "meta":
				meta = ""
				for _, e := range t.Attr {
					if e.Name.Local == "id" {
						meta = e.Value
						break
					}
				}
				state.inMeta = native_use[meta]
			case "whitespace":
				if len(words) > 0 {
					doFixed(fixedDone, nativeDone, hasMeta)
					fixedDone = true
					nativeDone = true
					if state.speaker != currentspeaker {
						doSpeaker(state.speaker, hasMeta, values, doc)
						currentspeaker = state.speaker
					}
					fmt.Fprintf(fpout, "%s|%s\n", label, strings.Join(words, " "))
					words = words[0:0]
					label += ".b"
					lineno++
					if *opt_m > 0 && *opt_m == lineno {
						break PARSE
					}
				}
			case "s", "utt":
				teller++
				label = fmt.Sprintf("%ss.%d", filename, teller)
				for _, e := range t.Attr {
					if e.Name.Local == "id" {
						label = e.Value
						break
					}
				}
				state.inS = true
				state.inW = false
				state.inT = false
			case "w":
				state.inW = true
				state.inT = false
			case "t":
				state.inT = true
			case "original", "morphology", "suggestion":
				state.inSkip = true
			}

			if _, ok := tt.(xml.EndElement); !ok {
				statestack = append(statestack, state)
			}
		} else if t, ok := tt.(xml.EndElement); ok {
			state := statestack[len(statestack)-1]
			statestack = statestack[0 : len(statestack)-1]
			switch t.Name.Local {
			case "s", "utt":
				if state.inS {
					if len(words) > 0 {
						doFixed(fixedDone, nativeDone, hasMeta)
						fixedDone = true
						nativeDone = true
						if state.speaker != currentspeaker {
							doSpeaker(state.speaker, hasMeta, values, doc)
							currentspeaker = state.speaker
						}
						fmt.Fprintf(fpout, "%s|%s\n", label, strings.Join(words, " "))
						words = words[0:0]
						lineno++
						if *opt_m > 0 && *opt_m == lineno {
							break PARSE
						}
					}
				}
			}
		} else if t, ok := tt.(xml.CharData); ok {
			state := statestack[len(statestack)-1]
			if state.inMetadata && state.inMeta {
				fmt.Fprintf(fpout, "##META %s %s = %s\n", native_items[meta].Type, native_items[meta].Label, string(t))
				native_seen[meta] = true
			}
			if state.inS && state.inT && !state.inSkip && (state.inW && cfg.Tokenized || !state.inW && !cfg.Tokenized) {
				for _, w := range strings.Fields(string(t)) {
					words = append(words, alpinoEscape(w))
				}
			}
		}
	}
	fmt.Fprintln(fpout)
	if cfg.Output_dir != "" {
		doEnd()
		native_seen = make(map[string]bool)
		doFixed(false, false, false)
	}
}

func doEnd() {
	if cfg.File_src != "" {
		fmt.Fprintf(fpout, "##META text %s =\n", cfg.File_src)
	}

	if cfg.File_path != "" {
		for i := 0; i < pathlevel; i++ {
			fmt.Fprintf(fpout, "##META text %s%d =\n", cfg.File_path, i+1)
		}
	}
}

func doFixed(metadone, nativedone, hasMeta bool) {

	if !metadone {
		if cfg.Meta_src != "" {
			fmt.Fprintf(fpout, "##META text %s =\n", cfg.Meta_src)
		}

		for _, item := range fixed {
			fmt.Fprintf(fpout, "##META %s %s =\n", cfg.Items[item].Type, item)
		}
		if !hasMeta {
			for _, item := range nonfixed {
				fmt.Fprintf(fpout, "##META %s %s =\n", cfg.Items[item].Type, item)
			}
		}
	}

	if !nativedone {
		for _, item := range native {
			if !native_seen[item] {
				fmt.Fprintf(fpout, "##META %s %s =\n", native_items[item].Type, native_items[item].Label)
			}
		}
	}
}

func doSpeaker(speaker string, hasMeta bool, values map[string]map[string][]string, doc *etree.Document) {
	if !hasMeta || len(nonfixed) == 0 {
		return
	}
	if _, ok := values[speaker]; !ok {
		values[speaker] = make(map[string][]string)
		for _, item := range nonfixed {
			found := false
			xpath := strings.Replace(cfg.Items[item].XPath, "%speaker%", speaker, -1)
			for _, t := range doc.FindElements(xpath) {
				found = true
				value := t.Text()
				if value != "" && oktype(item, value) {
					if _, ok := values[speaker][item]; !ok {
						values[speaker][item] = make([]string, 0, 1)
					}
					values[speaker][item] = append(values[speaker][item], value)
				}
			}
			if !found && speaker != "" {
				fmt.Fprintf(os.Stderr, "Niet gevonden in %s voor (%s) %s, %q\n", currentfile, cfg.Items[item].Type, item, speaker)
			}
		}
	}
	for _, item := range nonfixed {
		ii, ok := values[speaker][item]
		if !ok || len(ii) == 0 {
			fmt.Fprintf(fpout, "##META %s %s =\n", cfg.Items[item].Type, item)
			continue
		}
		for _, i := range ii {
			fmt.Fprintf(fpout, "##META %s %s = %s\n", cfg.Items[item].Type, item, i)
		}
	}
}

func oktype(item, value string) bool {
	var err error
	switch cfg.Items[item].Type {
	case "text":
	case "int":
		_, err = strconv.Atoi(value)
	case "float":
		_, err = strconv.ParseFloat(value, 32)
	case "date":
		var t time.Time
		t, err = time.Parse("2006-01-02", value)
		if err == nil {
			year := t.Year()
			if year < 1000 || year > 9999 {
				err = fmt.Errorf("Jaartal niet in bereik 1000 - 9999: %d", year)
			}
		}
	case "datetime":
		var t time.Time
		t, err = time.Parse("2006-01-02 15:04", value)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", value)
		}
		if err == nil {
			year := t.Year()
			if year < 1000 || year > 9999 {
				err = fmt.Errorf("Jaartal niet in bereik 1000-9999: %d", year)
			}
		}
	default:
		log.Fatalf("Fout in %s voor %s: onbekend type %q\n", currentfile, item, cfg.Items[item].Type)
	}
	if err == nil {
		return true
	}
	fmt.Fprintf(os.Stderr, "Fout in %s voor (%s) %s: %v\n", currentfile, cfg.Items[item].Type, item, err)
	return false
}

func alpinoEscape(s string) string {
	if cfg.Tokenized {
		switch s {
		case `[`:
			return `\[`
		case `]`:
			return `\]`
		case `\[`:
			return `\\[`
		case `\]`:
			return `\\]`
		}
	}
	return s
}

package main

import (
	"github.com/pebbe/util"

	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type Response struct {
	Code     int
	Status   string
	Message  string
	Id       string
	Interval int
	Finished bool
	Batch    []Line
}

type Line struct {
	Status   string
	Lineno   int
	Label    string
	Sentence string
	Xml      string
	Log      string
}

var (
	opt_d = flag.String("d", "xml", "directory voor uitvoer")
	opt_e = flag.String("e", "half", "escape level: none / half / full")
	opt_l = flag.String("l", "false", "true: één zin per regel; false: doorlopende tekst")
	opt_L = flag.String("L", "doc", "prefix voor labels")
	opt_n = flag.Int("n", 0, "maximum aantal tokens per regel")
	opt_p = flag.String("p", "", "alternatieve parser")
	opt_s = flag.String("s", "", "URL van Alpino-server")
	opt_t = flag.Int("t", 900, "time-out in seconden per regel")
	opt_T = flag.String("T", "false", "true: zinnen zijn getokeniseerd")

	x = util.CheckErr
)

func usage() {
	fmt.Fprintf(os.Stderr, `
Syntax: %s [opties] datafile

Optie:

  -s string : Alpino-server, zie: https://github.com/rug-compling/alpino-api
              Als deze ontbreekt wordt een lokale versie van Alpino gebruikt

Zonder gebruik van Alpino-server:

  De tekst moet bestaan uit één zin per regel, getokeniseerd, met of zonder labels.

Met gebruik van Alpino-server:

  De tekst kan verschillende vormen hebben.

Overige opties:

  -d string : Directory waar uitvoer wordt geplaatst (default: xml)
  -n int    : Maximum aantal tokens per regel (default: 0 = geen limiet)
  -p string : Alternatieve parser, zoals qa (default: geen)
  -t int    : Time-out per regel (default: 900)

Opties alleen van toepassing bij gebruik Alpino-server:

  -e string : Escape level: none / half / full (default: half)
  -l bool   : true: één zin per regel; false: doorlopende tekst (default: false)
  -L string : Prefix voor labels (default: doc)
  -T bool   : true: zinnen zijn getokeniseerd (default: false)

`, os.Args[0])
}

func main() {

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		return
	}
	filename := flag.Arg(0)

	// PARSEN

	if *opt_s == "" {
		os.MkdirAll(*opt_d, 0777)
		var fpin, fpout *os.File
		var errval error
		tmpfile := filename + ".part"
		var maxtok, parser string
		if *opt_n > 0 {
			maxtok = fmt.Sprint("max_sentence_length=", *opt_n)
		}
		if *opt_p != "" {
			parser = "application_type=" + *opt_p
		}
		defer func() {
			if fpin != nil {
				fpin.Close()
			}
			if fpout != nil {
				fpout.Close()
			}
			os.Remove(tmpfile)
			if errval != io.EOF {
				x(errval)
			}
		}()
		fpin, errval = os.Open(filename)
		if errval != nil {
			return
		}
		rd := util.NewReaderSize(fpin, 5000)
		n := 0
		for {
			line, err := rd.ReadLineString()
			if err != nil && err != io.EOF {
				errval = err
				return
			}
			if err == nil && n == 0 {
				fpout, errval = os.Create(tmpfile)
				if errval != nil {
					return
				}
			}
			if err == nil {
				fmt.Fprintln(fpout, line)
				n++
			}
			if (err == io.EOF && n > 0) || n == 10000 {
				n = 0
				fpout.Close()
				fpout = nil
				cmd := exec.Command(
					"/bin/bash",
					"-c",
					fmt.Sprintf(
						"$ALPINO_HOME/bin/Alpino -veryfast -flag treebank %s debug=1 end_hook=xml user_max=%d %s %s -parse < %s",
						*opt_d, *opt_t*1000, maxtok, parser, tmpfile))
				cmd.Env = []string{
					"ALPINO_HOME=" + os.Getenv("ALPINO_HOME"),
					"PATH=" + os.Getenv("ALPINO_HOME") + "/bin:" + os.Getenv("PATH"),
					"LANG=en_US.utf8",
					"LANGUAGE=en_US.utf8",
					"LC_ALL=en_US.utf8",
				}
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout
				errval = cmd.Run()
				if errval != nil {
					return
				}
			}
			if err == io.EOF {
				break
			}
		}
	} else {
		var buf bytes.Buffer
		fmt.Fprintf(
			&buf,
			`{"request":"parse", "lines":%v, "tokens":%v, "escape":%q, "label":%q, "timeout":%d, "parser":%q, "maxtokens":%d}`,
			*opt_l == "true",
			*opt_T == "true",
			*opt_e,
			*opt_L,
			*opt_t,
			*opt_p,
			*opt_n)
		fp, err := os.Open(filename)
		x(err)
		_, err = io.Copy(&buf, fp)
		fp.Close()
		x(err)
		resp, err := http.Post(*opt_s, "application/json", &buf)
		util.CheckErr(err)
		data, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		util.CheckErr(err)
		var response Response
		err = json.Unmarshal(data, &response)
		util.CheckErr(err)
		if response.Code > 299 {
			x(fmt.Errorf("%d %s -- %s", response.Code, response.Status, response.Message))
		}
		maxinterval := response.Interval
		id := response.Id

		go func() {
			chSignal := make(chan os.Signal, 1)
			signal.Notify(chSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
			sig := <-chSignal
			fmt.Fprintf(os.Stderr, "Signal: %v\n", sig)

			var buf bytes.Buffer
			fmt.Fprintf(&buf, `{"request":"cancel", "id":%q}`, id)
			resp, err := http.Post(*opt_s, "application/json", &buf)
			util.CheckErr(err)
			_, err = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			util.CheckErr(err)

			os.Exit(0)
		}()

		interval := 2
		for {
			if interval > maxinterval {
				interval = maxinterval
			}
			if interval > 120 {
				interval = 120
			}
			time.Sleep(time.Duration(interval) * time.Second)

			var buf bytes.Buffer
			fmt.Fprintf(&buf, `{"request":"output", "id":%q}`, id)
			resp, err := http.Post(*opt_s, "application/json", &buf)
			util.CheckErr(err)
			data, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			util.CheckErr(err)
			var response Response
			err = json.Unmarshal(data, &response)
			util.CheckErr(err)
			if response.Code > 299 {
				x(fmt.Errorf("%d %s -- %s", response.Code, response.Status, response.Message))
			}
			var lastdir string
			for _, line := range response.Batch {
				if line.Status == "ok" {
					if line.Label == "" {
						line.Label = fmt.Sprint(line.Lineno)
					}
					filename := filepath.Join(*opt_d, line.Label+".xml")
					dirname := filepath.Dir(filename)
					if dirname != lastdir {
						lastdir = dirname
						os.MkdirAll(dirname, 0777)
					}
					fp, err := os.Create(filepath.Join(*opt_d, line.Label+".xml"))
					x(err)
					fmt.Fprintln(fp, line.Xml)
					fp.Close()
				} else {
					fmt.Fprintf(os.Stderr, `**** parsing %s (line number %d)
%s
Q#%s|%s|%s|??|????
**** parsed %s (line number %d)
`,
						line.Label, line.Lineno,
						line.Log,
						line.Label, line.Sentence, line.Status,
						line.Label, line.Lineno)
				}
			}

			if response.Finished {
				break
			}
			interval = (3 * interval) / 2
		}
	}
}

package main

import (
	"github.com/pebbe/util"

	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

type Splitter struct {
	r io.Reader
}

var (
	opt_a = flag.String("a", "", "ALPINO_HOME")
	opt_d = flag.String("d", "xml", "Directory voor uitvoer")
	opt_s = flag.String("s", "", "Alpino server")
	opt_t = flag.Int("t", 900, "Time-out in seconden per regel")
)

func usage() {
	fmt.Fprintf(os.Stderr, `
Syntax: %s [opties] datafile

Verplichte optie:
  -a directory : ALPINO_HOME

Overige opties:
  -d directory : Directory waar uitvoer wordt geplaatst (default: xml)
  -s server    : Alpino-server ZONDER TOKENISATIE, als deze ontbreekt wordt
                 een lokale versie van Alpino gebruikt
  -t seconden  : Time-out per regel (default: 900), geen effect in combinatie met -s

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

	if *opt_a == "" {
		fmt.Println("Optie -a ontbreekt")
		return
	}

	os.Mkdir(*opt_d, 0777)

	// PARSEN

	if *opt_s == "" {
		cmd := exec.Command(
			"/bin/bash",
			"-c",
			fmt.Sprintf(
				"$ALPINO_HOME/bin/Alpino -veryfast -flag treebank %s debug=1 end_hook=xml user_max=%d -parse < %s",
				*opt_d, *opt_t * 1000, filename))
		cmd.Env = []string{
			"ALPINO_HOME=" + *opt_a,
			"PATH=" + os.Getenv("PATH"),
			"LANG=en_US.utf8",
			"LANGUAGE=en_US.utf8",
			"LC_ALL=en_US.utf8",
		}
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		util.CheckErr(cmd.Run())
	} else {
		cmd := exec.Command(
			"/usr/bin/curl", "-s", "--upload-file", filename, *opt_s)
		cmd.Stderr = os.Stderr
		reader, err := cmd.StdoutPipe()
		util.CheckErr(err)
		util.CheckErr(cmd.Start())
		var fp *os.File
		opened := false
		var topline string
		lineread := util.NewReader(reader)
		for {
			line, err := lineread.ReadLineString()
			if err == io.EOF {
				break
			}
			util.CheckErr(err)
			if opened {
				fmt.Fprintln(fp, line)
				if strings.HasPrefix(line, "</alpino_ds") {
					fp.Close()
					opened = false
				}
			} else {
				if strings.HasPrefix(line, "<alpino_ds") {
					a := strings.Split(line, " id=\"")[1]
					a = strings.Split(a, "\"")[0]
					a = a[strings.LastIndex(a, ".")+1:]
					n, err := strconv.Atoi(a)
					util.CheckErr(err)
					fname := path.Join(*opt_d, fmt.Sprintf("%08d.xml", n))
					fp, err = os.Create(fname)
					util.CheckErr(err)
					fmt.Fprintln(fp, topline)
					fmt.Fprintln(fp, line)
					opened = true
				} else if strings.HasPrefix(line, "<?xml") {
					topline = line
				} else {
					fmt.Fprintln(os.Stderr, line)
				}
			}
		}
		util.CheckErr(cmd.Wait())
	}
}
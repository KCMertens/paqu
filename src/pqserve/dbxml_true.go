// +build !nodbxml

package main

import (
	"github.com/pebbe/dbxml"

	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

const (
	has_dbxml = true
)

func get_dact(archive, filename string) ([]byte, error) {
	reader, err := dbxml.Open(archive)
	if err != nil {
		return []byte{}, err
	}
	defer reader.Close()
	d, err := reader.Get(filename)
	if err != nil {
		return []byte{}, err
	}
	return []byte(d), nil
}

func makeDact(dact, xml string, stripchar string, chKill chan bool) error {
	files, err := filenames2(xml)
	if err != nil {
		return err
	}

	os.Remove(dact)
	db, err := dbxml.Open(dact)
	if err != nil {
		return err
	}
	defer db.Close()

	for _, name := range files {

		select {
		case <-chGlobalExit:
			return errGlobalExit
		case <-chKill:
			return errKilled
		default:
		}

		data, err := ioutil.ReadFile(path.Join(xml, name))
		if err != nil {
			return err
		}

		name = decode_filename(name)
		if stripchar != "" {
			name = name[1+strings.Index(name, stripchar):]
		}
		err = db.PutXml(name, string(data), false)
		if err != nil {
			return err
		}
	}
	return nil
}

func unpackDact(data, xmldir, dact, stderr string, chKill chan bool) (tokens, nline int, err error) {

	os.Mkdir(xmldir, 0777)

	dc, err := dbxml.Open(dact)
	if err != nil {
		return 0, 0, fmt.Errorf(
			"Openen mislukt. PaQu kan geen dact-bestanden lezen die gemaakt zijn met DbXml nieuwer dan versie %d",
			dbxml_version_major())
	}
	defer dc.Close()

	docs, err := dc.All()
	if err != nil {
		return 0, 0, fmt.Errorf("Lezen dact-bestand: %s", err)
	}
	defer docs.Close()

	fperr, err := os.Create(stderr)
	if err != nil {
		return 0, 0, err
	}
	defer fperr.Close()

	fplines, err := os.Create(data + ".lines")
	if err != nil {
		return 0, 0, err
	}
	defer fplines.Close()

	tokens = 0
	nline = 0
	nd := -1
	sdir := ""
	for docs.Next() {

		select {
		case <-chGlobalExit:
			return 0, 0, errGlobalExit
		case <-chKill:
			return 0, 0, errKilled
		default:
		}

		nline++

		if nline%10000 == 1 {
			nd++
			sdir = fmt.Sprintf("%04d", nd)
			os.Mkdir(path.Join(xmldir, sdir), 0777)
		}

		name := docs.Name()
		if strings.HasSuffix(strings.ToLower(name), ".xml") {
			name = name[:len(name)-4]
		}
		encname := encode_filename(name)

		data := []byte(docs.Content())

		fp, err := os.Create(path.Join(xmldir, sdir, encname+".xml"))
		if err != nil {
			return 0, 0, err
		}
		_, err = fp.Write(data)
		fp.Close()
		if err != nil {
			return 0, 0, err
		}

		alpino := Alpino_ds_no_node{}
		err = xml.Unmarshal(data, &alpino)
		if err != nil {
			return 0, 0, fmt.Errorf("Parsen van %q uit dact-bestand: %s", docs.Name(), err)
		}
		tokens += len(strings.Fields(alpino.Sentence))
		fmt.Fprintf(fplines, "%s|%s\n", name, strings.TrimSpace(alpino.Sentence))
		for _, c := range alpino.Comments {
			if strings.HasPrefix(c.Comment, "Q#") {
				a := strings.SplitN(c.Comment, "|", 2)
				if len(a) == 2 {
					fmt.Fprintf(fperr, "Q#%s|%s\n", name, strings.TrimSpace(a[1]))
					break
				}
			}
		}
	}

	return tokens, nline, nil
}

func dbxml_version() string {
	x, y, z := dbxml.Version()
	return fmt.Sprintf("Version %d.%d.%d", x, y, z)
}

func dbxml_version_major() int {
	x, _, _ := dbxml.Version()
	return x
}

package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"strings"

	"github.com/pkg/errors"
)

var templates = template.Must(ParseFSStrict(resources, "templates"))

// ParseFSStrict works like template.ParseFS, but is more strict:
//
// - each filename can contain only one template and may not {{define}} subtemplates
// - filenames must end in .tmpl
// - the template will be given the name of the file, without the extension
//
// The returned template's name will have the base name and parsed contents of the first file. There must be at least one file. If an error occurs, parsing stops and the returned *Template is nil.

// TODO: submit pull request to add this to the html/template library.

func ParseFSStrict(resources fs.FS, dirs ...string) (*template.Template, error) {
	var ts *template.Template
	for _, dir := range dirs {

		templateFiles, err := fs.ReadDir(resources, dir)
		if err != nil {
			return ts, errors.Wrap(err, "fs.ReadDir("+dir+")")
		}

		for _, dirEntry := range templateFiles {
			if dirEntry.IsDir() {
				continue
			}
			fileName := dirEntry.Name()
			f := strings.Split(fileName, ".")
			if len(f) > 0 {
				if f[len(f)-1] != "tmpl" {
					continue
				}
				templateName := f[0]
				t := template.Must(template.ParseFS(resources, dir+"/"+fileName))

				for _, subTemplate := range t.Templates() {
					if subTemplate.Name() != fileName {
						return ts, fmt.Errorf(`{{define "%v"}} in file %v not allowed when using ParseFSStrict. Each template file must contain one template whose name matches the filename.`, subTemplate.Name(), fileName)
					}
				}

				// The returned template's name will have the base name and parsed contents of the first file
				if ts == nil {
					ts = t
				}

				_, err := ts.AddParseTree(templateName, t.Tree)
				if err != nil {
					return ts, errors.Wrap(err, "ts.AddParseTree("+templateName+")")
				}
			}
		}
	}
	return ts, nil
}

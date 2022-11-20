package main

import (
	"fmt"
	"html/template"
	"io/fs"

	"github.com/pkg/errors"
	// some templates functions we might use
	// "github.com/Masterminds/sprig/v3"
)

var templates = template.Must(ParseFSStrict(resources, "templates"))

// ParseFSStrict works like template.ParseFS, but is more strict:
// - each template will be given the same name as the file it is defined in
// - each filename can contain only one template and may not {{define}} subtemplates
// - filenames must end in .tmpl
//
// This approach eliminates the possibility of inconsistency between the names
// of templates and the names of template files, reducing decision overhead
// and opportunities for surprises for developers. It also eliminates the
// possibility of two templates accidentally being given the same name, which
// will result in one template being overwritten by the other and can create
// surprising bugs (this was the immediate motivation for creating this
// function).
//
// The returned template's name will have the base name and parsed contents of
// the first file. There must be at least one file. If an error occurs,
// parsing stops and the returned *Template is nil.
//
// Templates in subdirectories of the provided directory will be parsed. The
// names of templates in subdirectories will be prefixed with the name
// of subdirectory (e.g. "charts/chart1.html.tmpl")
//
// TODO: submit pull request to add this to the html/template library.

func ParseFSStrict(resources fs.FS, dir string) (*template.Template, error) {
	var ts *template.Template

	templateFiles, err := fs.ReadDir(resources, dir)
	if err != nil {
		return ts, errors.Wrapf(err, "fs.ReadDir(%s)", dir)
	}

	for _, dirEntry := range templateFiles {
		if dirEntry.IsDir() {
			subDirName := dir + "/" + dirEntry.Name()
			subTemplates, err := ParseFSStrict(resources, subDirName)
			if err != nil {
				return ts, errors.Wrapf(err, "fs.ReadDir(%s)", subDirName)
			}

			for _, t := range subTemplates.Templates() {
				fileName := dirEntry.Name() + "/" + t.Name()
				if ts == nil {
					ts = t
				}
				_, err := ts.AddParseTree(fileName, t.Tree)
				if err != nil {
					return ts, errors.Wrapf(err, "ts.AddParseTree(%s)", fileName)
				}
			}

			continue
		}
		fileName := dirEntry.Name()

		// use this to add sprig functions.
		// t, err := template.New(fileName).Funcs(sprig.FuncMap()).ParseFS(resources, dir+"/"+fileName)
		t, err := template.New(fileName).ParseFS(resources, dir+"/"+fileName)
		if err != nil {
			return ts, errors.Wrapf(err, "parsing template %s", dir+"/"+fileName)
		}

		for _, t := range t.Templates() {
			if t.Name() != fileName {
				return ts, fmt.Errorf(`{{define "%v"}} in file %v not allowed when using ParseFSStrict. Each template file must contain one template whose name will be equal to the filename.`, t.Name(), fileName)
			}
		}
		if ts == nil {
			ts = t
		}

		_, err = ts.AddParseTree(fileName, t.Tree)
		if err != nil {
			return ts, errors.Wrapf(err, "ts.AddParseTree(%s)", fileName)
		}
	}

	if ts == nil {
		return ts, fmt.Errorf("No template files found in directory %s", dir)
	}
	return ts, nil
}

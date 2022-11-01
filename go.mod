module github.com/social-protocols/news

go 1.18

require (
	github.com/dustin/go-humanize v1.0.0
	github.com/go-kit/log v0.2.1
	github.com/gorilla/schema v1.2.0
	github.com/hashicorp/go-retryablehttp v0.7.1
	github.com/johnwarden/hn v1.0.0
	github.com/johnwarden/httperror/v2 v2.3.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mattn/go-sqlite3 v1.14.15
	github.com/multiprocessio/go-sqlite3-stdlib v0.0.0-20220822170115-9f6825a1cd25
	github.com/pkg/errors v0.9.1
	gonum.org/v1/plot v0.12.0
)

//replace github.com/johnwarden/httperror/v2 v2.3.0 => ../httperror

//replace "github.com/johnwarden/hn" v1.0.0 => "../hn"

require (
	git.sr.ht/~sbinet/gg v0.3.1 // indirect
	github.com/ajstarks/svgo v0.0.0-20211024235047-1546f124cd8b // indirect
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de // indirect
	github.com/fatih/color v1.12.0 // indirect
	github.com/go-fonts/liberation v0.2.0 // indirect
	github.com/go-latex/latex v0.0.0-20210823091927-c0d11ff05a81 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-pdf/fpdf v0.6.0 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v0.16.2 // indirect
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	golang.org/x/crypto v0.0.0-20220926161630-eccd6366d1be // indirect
	golang.org/x/image v0.0.0-20220902085622-e7cb96979f69 // indirect
	golang.org/x/sys v0.0.0-20220919091848-fb04ddd9f9c8 // indirect
	golang.org/x/text v0.3.7 // indirect
	gonum.org/v1/gonum v0.12.0 // indirect
)

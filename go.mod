module github.com/social-protocols/news

go 1.22

toolchain go1.23.3

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/VictoriaMetrics/metrics v1.23.0
	github.com/dustin/go-humanize v1.0.1
	github.com/gocolly/colly/v2 v2.1.0
	github.com/gorilla/schema v1.2.0
	github.com/hashicorp/go-retryablehttp v0.7.1
	github.com/johnwarden/hn v1.0.1
	github.com/johnwarden/httperror v1.6.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mattn/go-sqlite3 v1.14.15
	github.com/minio/minio-go/v7 v7.0.80
	github.com/multiprocessio/go-sqlite3-stdlib v0.0.0-20220822170115-9f6825a1cd25
	github.com/pkg/errors v0.9.1
	github.com/weppos/publicsuffix-go v0.20.0
	golang.org/x/exp v0.0.0-20221114191408-850992195362
	gonum.org/v1/gonum v0.12.0
	gorm.io/driver/sqlite v1.4.3
	gorm.io/gorm v1.24.2
)

//replace github.com/johnwarden/httperror v1.6.0 => ../httperror
//replace "github.com/johnwarden/hn" v1.0.1 => "../hn"

require (
	github.com/PuerkitoBio/goquery v1.5.1 // indirect
	github.com/andybalholm/cascadia v1.2.0 // indirect
	github.com/antchfx/htmlquery v1.2.3 // indirect
	github.com/antchfx/xmlquery v1.2.4 // indirect
	github.com/antchfx/xpath v1.1.8 // indirect
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v0.16.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kennygrant/sanitize v1.2.4 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/saintfish/chardet v0.0.0-20120816061221-3af4cd4741ca // indirect
	github.com/temoto/robotstxt v1.1.1 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	golang.org/x/crypto v0.28.0 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/protobuf v1.24.0 // indirect
)

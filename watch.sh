ls *.go **/**.tmpl **/**.sql | entr -ncr sh -c 'go install; go run *.go' 

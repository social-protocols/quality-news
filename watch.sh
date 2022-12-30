if which humanlog ; then
	LOGFORMATTER="| humanlog --truncate=0"
fi

ls *.go **/**.tmpl **/**.sql | entr -ncr sh -c "go install; go run *.go $LOGFORMATTER"

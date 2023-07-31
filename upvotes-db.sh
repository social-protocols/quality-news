# Open the upvotes DB as read-write, then attach the frontpage DB as readonly.

# We need to pass an init script filename to sqlite3 to run the attach command at the beginning of the shell session.
initscript=$(mktemp /tmp/init-db.XXXXXX)
echo "attach database 'file:/Users/jwarden/hacker-news-data-datadir/frontpage.sqlite?mode=ro' as frontpage;
.mode column
.header on
" > $initscript 

# Delete the tempfile after sqltie has tarted
(sleep 1 && rm "$initscript")&

sqlite3 $SQLITE_DATA_DIR/upvotes.sqlite --init $initscript


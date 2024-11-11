set dotenv-load := true

# List available recipes in the order in which they appear in this file
_default:
    @just --list --unsorted

watch:
	./watch.sh

sqlite:
	sqlite3 $SQLITE_DATA_DIR/frontpage.sqlite

upvotes-db:
	./upvotes-db.sh

format:
	go fmt
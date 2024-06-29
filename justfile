# List available recipes in the order in which they appear in this file
_default:
    @just --list --unsorted

dev:
    ./watch.sh

db:
    sqlite3 $SQLITE_DATA_DIR/frontpage.sqlite

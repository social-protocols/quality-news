#!/bin/bash
set -e  # Exit on any error

# Configuration
APP_NAME="news"
OLD_VOLUME_NAME="data2"
NEW_VOLUME_NAME="data3"
NEW_VOLUME_SIZE="3"  # Adjust this to your needs
REGION="ewr"  # Your current region

# Function to wait for VM to be ready
wait_for_vm() {
    echo "Waiting for VM to be ready..."
    while true; do
        STATUS=$(fly status --app $APP_NAME)
        if echo "$STATUS" | grep -q "running"; then
            echo "VM is ready"
            break
        fi
        echo "VM not ready yet, waiting..."
        sleep 5
    done
}

echo "Stopping the application..."
fly scale count 0 --app $APP_NAME

echo "Creating new volume..."
fly volumes create $NEW_VOLUME_NAME --size $NEW_VOLUME_SIZE --region $REGION 

echo "Creating temporary machine with old volume..."
cat > migrate-old.toml << EOL
app = "$APP_NAME"
primary_region = "$REGION"

[build]
  image = "alpine:latest"

[mounts]
  source = "$OLD_VOLUME_NAME"
  destination = "/data"

[processes]
  app = "sleep infinity"
EOL

echo "Deploying temporary machine with old volume..."
fly deploy --config migrate-old.toml --app $APP_NAME
wait_for_vm

echo "Copying data from old volume to temporary storage..."
fly ssh console --command 'cd /data && tar czf frontpage.sqlite.gz frontpage.sqlite && tar czf frontpage.sqlite-shm.gz frontpage.sqlite-shm && tar czf frontpage.sqlite-wal.gz frontpage.sqlite-wal' --app $APP_NAME

echo "Downloading database files from old volume..."
fly sftp shell --app $APP_NAME << EOF
get /data/frontpage.sqlite.gz ~/social-protocols-data/recover/frontpage.sqlite.gz
get /data/frontpage.sqlite-shm.gz ~/social-protocols-data/recover/frontpage.sqlite-shm.gz
get /data/frontpage.sqlite-wal.gz ~/social-protocols-data/recover/frontpage.sqlite-wal.gz
exit
EOF

echo "Destroying temporary machine..."
fly scale count 0 --app $APP_NAME
fly machines destroy $(fly machines list --json | jq -r '.[].id') --force --app $APP_NAME

echo "Creating temporary machine with new volume..."
cat > migrate-new.toml << EOL
app = "$APP_NAME"
primary_region = "$REGION"

[build]
  image = "alpine:latest"

[mounts]
  source = "$NEW_VOLUME_NAME"
  destination = "/data"

[processes]
  app = "sleep infinity"
EOL

echo "Deploying temporary machine with new volume..."
fly deploy --config migrate-new.toml --app $APP_NAME
wait_for_vm

echo "Uploading database files to new volume..."
fly sftp shell --app $APP_NAME << EOF
put ~/social-protocols-data/recover/frontpage.sqlite.gz /data/frontpage.sqlite.gz
put ~/social-protocols-data/recover/frontpage.sqlite-shm.gz /data/frontpage.sqlite-shm.gz
put ~/social-protocols-data/recover/frontpage.sqlite-wal.gz /data/frontpage.sqlite-wal.gz
exit
EOF

echo "Extracting database files on new volume..."
fly ssh console --command 'cd /data && gunzip frontpage.sqlite.gz && gunzip frontpage.sqlite-shm.gz && gunzip frontpage.sqlite-wal.gz' --app $APP_NAME

echo "Updating mount configuration..."
# Create a temporary file for the new fly.toml
cat > fly.toml.new << EOL
[mounts]
  source = "$NEW_VOLUME_NAME"
  destination = "/data"
EOL

# Backup the original fly.toml
cp fly.toml fly.toml.backup

# Update the mounts section in fly.toml
sed -i.bak '/\[mounts\]/,/^$/c\' fly.toml
cat fly.toml.new >> fly.toml
rm fly.toml.new migrate-old.toml migrate-new.toml

echo "Deploying application with new volume..."
fly deploy
wait_for_vm

echo "Verifying application is running..."
fly status --app $APP_NAME

echo "If everything looks good, you can delete the old volume with:"
echo "fly volumes delete $OLD_VOLUME_NAME --app $APP_NAME"
echo ""
echo "To rollback, restore the original fly.toml:"
echo "mv fly.toml.backup fly.toml" 
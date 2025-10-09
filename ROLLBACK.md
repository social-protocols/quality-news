# Emergency Rollback Instructions

If the worker split causes issues in production, follow these steps to quickly rollback.

## Quick Rollback (1 minute)

```bash
# Merge the rollback branch to master
git checkout master
git merge rollback-worker-split --no-edit
git push origin master

# Deploy will automatically trigger via CI
```

This reverts the code to the original single `archiveWorker` implementation.

## What Gets Reverted

- ✅ `archive.go` - Back to single archiveWorker handling both operations
- ✅ `main.go` - Back to starting only archiveWorker
- ✅ `app.go` - Back to original comment
- ✅ `DISK-USAGE-FIX.md` - Removed

## After Rollback

The system will return to the previous behavior:
- Single archiveWorker handling both archiving and purging
- Purge operations will be blocked by archiving again
- "Archive trigger channel full" warnings will likely resume
- Database will continue to accumulate old data

## If You Need to Retry the Fix

After investigating and addressing any issues:

```bash
# Delete the rollback branch
git branch -D rollback-worker-split
git push origin --delete rollback-worker-split

# Reapply the fix (from master, which has the fix)
git checkout master
# ... investigate and fix any issues ...
# ... apply updated fix ...
```

## Monitoring After Rollback

Check logs to confirm rollback worked:
```bash
fly logs -a social-protocols-news
```

Should see:
- `"Archive worker shutting down"` (from both workers)
- Then only single `"Archive worker started"` after restart
- System continues to operate (crawls run normally)

---

**Note:** The rollback branch `rollback-worker-split` is already pushed to origin and ready to merge at any time.

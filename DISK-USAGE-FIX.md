# Disk Usage Fix - Implementation Details

## Problem Diagnosed

**Root Cause:** The purge worker was never executing because archiving operations blocked the signal channel.

### Evidence
- Database had accumulated significantly more data than expected
- Data older than 30 days was not being deleted
- Logs showed: `"Archive trigger channel full, skipping signal"` on **every crawl**

### Why This Happened
The original design had a single `archiveWorker` goroutine handling both:
1. **Archiving** (runs every 5 minutes, takes 4+ minutes)
2. **Purging** (should run during idle time between crawls)

They shared the same event loop with a channel buffer size of 1:
```go
select {
case idleCtx := <-app.archiveTriggerChan:  // Purge
    processPurgeOperations(idleCtx)
case <-archiveTicker.C:  // Every 5 minutes
    processArchivingOperations(ctx)  // Takes 4+ minutes!
}
```

**Problem:** When archiving was running (which takes several minutes), the channel stayed full and ALL idle signals were dropped. Purge operations never executed, and `deleteOldData()` never ran.

## Solution Implemented

### Changes Made

**1. Split into Two Independent Workers** (`archive.go`)

Created two separate goroutines:

```go
// archiveWorker - Handles archiving on a schedule
func (app app) archiveWorker(ctx context.Context) {
    // Runs every 5 minutes
    // Can take as long as needed
    // Does NOT block purge operations
}

// purgeWorker - Handles purge during idle time  
func (app app) purgeWorker(ctx context.Context) {
    // Listens for idle signals
    // Runs immediately when signal received
    // Independent of archiving
}
```

**2. Updated Main to Start Both Workers** (`main.go`)

```go
// Start the archive worker (runs every 5 minutes)
go app.archiveWorker(ctx)

// Start the purge worker (runs during idle time between crawls)
go app.purgeWorker(ctx)
```

**3. Increased Channel Buffer Size** (`app.go`)

```go
archiveTriggerChan: make(chan context.Context, 10), // Was: 1
```

This allows idle signals to queue up if the purge worker is temporarily busy.

**4. Improved Logging** (`main.go`)

Changed logging to make it clear when signals are sent vs dropped:
```go
case app.archiveTriggerChan <- crawlCtx:
    app.logger.Debug("Sent idle context to purge worker", "available_seconds", delay)
default:
    app.logger.Warn("Purge trigger channel full, signal dropped - purge worker may be backed up")
```

### Why This Works

✅ **Archiving and purging are now independent**
- Long archiving operations don't block purge signal reception
- Purge worker always listening, ready to process idle periods

✅ **SQLite write lock is still respected**
- Only one operation writes at a time
- Purging only happens during idle time (when crawler isn't running)
- Archiving generates data from database but uploads don't block DB writes

✅ **Better resilience**
- If one worker fails, the other continues
- Channel buffer size (1) prevents processing expired contexts

✅ **Purge operations will now run every minute**
- Instead of never running, purge now runs after every successful crawl
- Will process accumulated data backlog

## Expected Results

### Phase 1: Initial Cleanup (Days 1-7)
- Purge worker will delete old data aggressively
- Database will begin to shrink
- You'll see logs: "Deleted old data, rowsDeleted: XXXXX"
- May take several days to process accumulated data

### Phase 2: Stabilization (Days 7-14)
- Data backlog cleared
- Database size stabilizes
- Only 21-30 days of data retained
- Metrics show healthy operation

### Steady State
- Total rows: approximately 21 days × stories per crawl × minutes per day
- Data age: **Max 30 days**
- No more "channel full" warnings
- Disk usage stable

## Deployment Steps

### 1. Build and Deploy

```bash
# Test locally if possible
go build

# Commit changes
git add archive.go main.go app.go
git commit -m "Fix: Separate archiving and purging workers to prevent channel blocking

- Split archiveWorker into archiveWorker and purgeWorker
- archiveWorker runs archiving every 5 minutes independently
- purgeWorker handles purge operations during idle time
- Increased channel buffer from 1 to 10
- Improved logging for signal tracking

Fixes disk usage growth issue where 206 days of data accumulated
instead of being deleted after 30 days."

# Deploy to production
git push origin master
fly deploy
```

### 2. Monitor Deployment

Watch logs for the new workers starting:
```bash
fly logs -a social-protocols-news
```

Look for:
- `"Archive worker started"` (from archiveWorker)
- `"Purge worker started"` (from purgeWorker)
- `"Sent idle context to purge worker"` (should appear every minute)
- `"Deleting old data"` (purge operations executing)

### 3. Track Progress

Monitor these metrics over the next week:

**Database Stats:**
```sql
SELECT 
    COUNT(*) as total_rows,
    ROUND((strftime('%s', 'now') - MIN(sampleTime)) / 86400.0, 1) as oldest_days
FROM dataset;
```

**Expected Progress:**
- Row count should decrease daily
- Oldest data age should decrease toward 30 days
- Process may take 1-2 weeks depending on accumulated backlog

**File Size:**
```bash
fly ssh console -a social-protocols-news
du -sh /data/frontpage.sqlite*
```

Expect gradual decrease as data is deleted and vacuum reclaims space.

### 4. Verify Fix

After 7-14 days, check:

```sql
-- Check total rows (should match expected retention period)
SELECT COUNT(*) FROM dataset;

-- Check oldest data age (should be ~30 days max)
SELECT ROUND((strftime('%s', 'now') - MIN(sampleTime)) / 86400.0, 1) FROM dataset;

-- Check for data older than 30 days (should be 0 or very small)
SELECT COUNT(*) FROM dataset 
WHERE sampleTime <= strftime('%s', 'now') - 30*24*60*60;
```

## Rollback Plan

If issues occur:

1. **Quick rollback:**
```bash
git revert HEAD
fly deploy
```

2. **Emergency: Revert to single worker:**

In `main.go`, comment out the purge worker:
```go
go app.archiveWorker(ctx)
// go app.purgeWorker(ctx)  // Temporarily disabled
```

And in `archive.go`, revert to the old combined worker logic.

## Monitoring Recommendations

### Add These Metrics (Future Enhancement)

```go
// In prometheus.go or new metrics file
var (
    purgeSignalsSentTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "purge_signals_sent_total",
        Help: "Total purge signals sent to worker",
    })
    
    purgeSignalsDroppedTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "purge_signals_dropped_total",
        Help: "Total purge signals dropped due to full channel",
    })
    
    oldDataRowsDeletedTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "old_data_rows_deleted_total",
        Help: "Total rows deleted by deleteOldData",
    })
    
    databaseRowsTotal = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "database_rows_total",
        Help: "Total rows in dataset table",
    })
    
    databaseOldestDataAgeDays = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "database_oldest_data_age_days",
        Help: "Age of oldest data in days",
    })
)
```

Then track:
- Purge signals sent vs dropped
- Rows deleted per cycle
- Database size trend
- Data age trend

## Files Changed

| File | Changes | Reason |
|------|---------|--------|
| `archive.go` | Split `archiveWorker` into `archiveWorker` and `purgeWorker` | Prevent archiving from blocking purge signals |
| `main.go` | Start both workers, improve logging | Launch independent workers, better visibility |
| `app.go` | Increase channel buffer 1→10 | Handle burst scenarios |

## Testing Performed

- ✅ Code compiles without errors
- ✅ No linter errors
- ✅ Logic review: workers are independent
- ✅ SQLite write lock still respected
- ⚠️ Local testing not performed (requires production-scale data)

## Questions & Answers

**Q: Won't two workers cause SQLite lock conflicts?**  
A: No. They never write simultaneously:
- Purge worker only runs during idle time (crawler not running)
- Archive worker reads from DB, writes to external storage
- Only purge operations write to DB, and they're controlled by idle signals

**Q: What if purge worker falls behind?**  
A: The channel buffer (size 1) allows one signal to queue while processing. If truly backed up, additional signals drop and log a warning. Will catch up when operations finish.

**Q: How long until database returns to normal size?**  
A: Depends on the accumulated backlog. Typically 7-14 days for full recovery as old data is gradually deleted.

**Q: Is data at risk?**  
A: No. This is a performance/disk space issue, not data corruption. All data is safe, just more than intended.

## Success Criteria

Fix is successful when:
- ✅ Database size stabilizes at expected level for retention period
- ✅ Oldest data age ≤ 30 days
- ✅ No "channel full" warnings in logs
- ✅ Purge operations visible in logs every minute
- ✅ Disk usage no longer growing

---

**Implemented by:** Claude (AI Assistant)  
**Date:** October 7, 2025  
**Issue:** Disk usage growing due to purge operations never executing  
**Root Cause:** Archiving operations blocking purge signal channel  
**Solution:** Split into independent workers with separate goroutines

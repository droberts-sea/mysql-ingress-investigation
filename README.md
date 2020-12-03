# Mysql Ingress

I did some more testing running qbert volumes and costbasis volumes side-by-side today, and am seeing a frustratingly small discrepancy. Out of ~250k transactions from the beginning of April 2020, 825 have an `oldest_cancellation` time that is off by one second.

Other details:
- The qbert version is always 1 second after the costbasis version
- There are many records with an `oldest_cancellation` where the times match

This is not expected behavior, according to Rushabh

## Theory
Sounds like a rounding error!

Further evidence: There are 1602 records that have an oldest_cancellation time at all, and it would fit for about 50% to have a rounding error.

Assuming nothing has changed about reading / enriching events, I see two places it could have crept in:

- `LOAD DATA` handles rounding differently than `INSERT`
- passing `volume.OldestCancellation` to `tx.Exec` (old way) works differently than calling `v.OldestCancellation.String()` in `TableRecord` (new way)

### LOAD DATA vs INSERT

Initial excitement: "Bing bing bing looks like this is correct! LOAD DATA truncates, INSERT rounds off"

https://www.epochconverter.com/

Ah, no, not actually. LOAD DATA and INSERT both round - see test script (`ingress.sql`)

### Handoff to SQL

OK, next investigation. Maybe we’re handing the value to SQL differently.

#### Old way:

```go
es, err = tx.Exec(queries.Get("upsert-txn-volume"), volume.TransactionId, volume.OldestQueue, volume.LatestQueue, volume.CaptureCount, volume.UndoCaptureCount,
				volume.SendCurrencyCode, volume.PayoutCurrencyCode, volume.PayoutAmount, volume.OldestCancellation, volume.LatestQueue, volume.CaptureCount, volume.UndoCaptureCount,
        volume.SendCurrencyCode, volume.PayoutCurrencyCode, volume.PayoutAmount, volume.OldestCancellation)
```

So passing `volume.OldestCancellation` to `tx.Exec` directly

What function does `tx.Exec` call on the data it gets?

*Probably* it casts it to a time.Time?

#### New way:

```go
// Calls db.RecordsToBytes
buf, err := db.RecordsToBytes(txnVolume)

// Which calls Volume.TableRecord
// TableRecord implements db.TableRecord.
func (v Volume) TableRecord() []string {
	oldestCancellation := "\\N"
	if v.OldestCancellation != nil {
		oldestCancellation = v.OldestCancellation.String()
	}
	return []string{
		v.TransactionId,
		v.OldestQueue.String(),
		v.LatestQueue.String(),
		strconv.Itoa(v.CaptureCount),
		strconv.Itoa(v.UndoCaptureCount),
		v.SendCurrencyCode.String(),
		v.PayoutCurrencyCode.String(),
		v.PayoutAmount.StringFullPrecision(),
		oldestCancellation,
	}
}
```

So calling `volume.OldestCancellation.String()`.

```go
// Implementation of SqlTimestamp.String():
type SqlTimestamp time.Time

func (ts SqlTimestamp) String() string {
	return time.Time(ts).Format(SqlTimestampFormat)
}
```

Have confirmed via `./main.go`: calling `.String()` does **TRUNCATE** the timestamp.

#### Test: Insert into DB both ways

From Go, connect to database, do insert passing value direclty, do insert calling .String() first.

```go
conn := connect()
conn.Exec(
  "INSERT INTO test_table (label, time_seconds, time_millis) VALUES (?, ?, ?)",
  "Go INSERT pass direct",
  vol.OldestCancellation,
  vol.OldestCancellation,
)
conn.Exec(
  "INSERT INTO test_table (label, time_seconds, time_millis) VALUES (?, ?, ?)",
  "Go INSERT call .String()",
  vol.OldestCancellation.String(),
  vol.OldestCancellation.String(),
)
```

Yep, that's done it:

```
mysql> select * from test_table;
+--------------------------+---------------------+-------------------------+
| label                    | time_seconds        | time_millis             |
+--------------------------+---------------------+-------------------------+
| Go INSERT call .String() | 2020-04-02 05:16:08 | 2020-04-02 05:16:08.000 |
| Go INSERT pass direct    | 2020-04-02 05:16:09 | 2020-04-02 05:16:08.987 |
```

## Question: Why Not Other Timestamps?

There are several timestamps in the volume struct. Why are the others not hitting this issue?

```go
type Volume struct {
	TransactionId      string
	OldestQueue        cal.SqlTimestamp   // These have the same behavior in qbert and cb-etl
	LatestQueue        cal.SqlTimestamp   // no problem
	CaptureCount       int
	UndoCaptureCount   int
	SendCurrencyCode   fin.Currency
	PayoutCurrencyCode fin.Currency
	PayoutAmount       fin.Dollars
	OldestCancellation *cal.SqlTimestamp  // This truncates in cb but rounds in qbert
}
```

`cal.SqlTimestamp` is just an alias for `time.Time`, so internally we're at least capable of ms resolution

They all get treated the same way for both flows:

```go
// old: pass directly
res, err = tx.Exec(queries.Get("upsert-txn-volume"), volume.TransactionId, volume.OldestQueue, volume.LatestQueue, volume.CaptureCount, volume.UndoCaptureCount,
				volume.SendCurrencyCode, volume.PayoutCurrencyCode, volume.PayoutAmount, volume.OldestCancellation, volume.LatestQueue, volume.CaptureCount, volume.UndoCaptureCount,
        volume.SendCurrencyCode, volume.PayoutCurrencyCode, volume.PayoutAmount, volume.OldestCancellation)

// new: call .String()
func (v Volume) TableRecord() []string {
	oldestCancellation := "\\N"
	if v.OldestCancellation != nil {
		oldestCancellation = v.OldestCancellation.String()
	}
	return []string{
		v.TransactionId,
		v.OldestQueue.String(),
		v.LatestQueue.String(),
		strconv.Itoa(v.CaptureCount),
		strconv.Itoa(v.UndoCaptureCount),
		v.SendCurrencyCode.String(),
		v.PayoutCurrencyCode.String(),
		v.PayoutAmount.StringFullPrecision(),
		oldestCancellation,
	}
}
```

All fin event timestamps have millisecond resolution:

```json
$ cat f0b57c49-4e98-486d-88a3-6ee1e4b66d06.log | python -m json.tool
{
    "event_id": "f0b57c49-4e98-486d-88a3-6ee1e4b66d06",
    "event_name": "queue",
    "application_log_timestamp": "1585699429740", // HERE
    "app": "platform",
    "transaction_canonical_id": "1677e124019034f5ec28b0c2cc5fd959",
    "matching_id": "12800420511",
    "effective_time": "1585699429000",  // HERE
    "event_amount": "30199",
    "event_currency": "USD"
    // ... more fields, no more timestamps
}
```

Implication: we must be truncating the fine values (`OldestQueue` and `LatestQueue`) somewhere in the fin event -> volume workflow, but we're not doing the same thing for `OldestCancellation`.

### Checkout Revise.go

In `revise.go` (previoulsly `app/core/costbasis/volume/`, now `app/sotres/volume/`) all are converted as follows:

```go
txnLiabilityRecord.OldestQueue = cal.SqlTimestamp(event.ExtractAccountingImpactTime())
txnLiabilityRecord.LatestQueue = cal.SqlTimestamp(event.ExtractAccountingImpactTime())
sqlTime := cal.SqlTimestamp(event.ExtractAccountingImpactTime())
txnLiabilityRecord.OldestCancellation = &sqlTime
```

The basis for all three is `event.ExtractAccountingImpactTime()`, which should have the same resolution?

```go
// ExtractAccountingImpactTime gets the accounting impact time for the event.
func (fe FinEvent) ExtractAccountingImpactTime() cal.MillisTimestamp {
	return fe.EffectiveTime
}
```

Oh shit, different event types have different resolutions! Checkout the last 3 digits of `effective_time`!

```
# interesting_fin_events.log contains the raw UEL events for all problematic volumes
$ cat interesting_fin_events.log | grep '"transaction_canonical_id":"f9efcb347fd8840dd04944cedcd91cb6"' | cut -d "," -f 7,2
"event_name":"queue","effective_time":"1585851865000"
"event_name":"queue","effective_time":"1585851919000"
"event_name":"capture_funds","effective_time":"1585851929986"
"event_name":"cash_settle","effective_time":"1585918800000"
"event_name":"settle_refund","effective_time":"1585852001773"
```

`OldestCancellation` is taken from the `capture_funds` event, whereas `OldestQueue` and `LatestQueue` come from the `queue` (duh)

If we ever did get ms resolution on a queue event, we would see the same difference in behavior there.

## So what do we do about it?

You could argue that either behavior is correct:

- Old qbert advertises second granularity, but if someone gives us higher granularity, we'll keep track of it in code and round it off when we go to the DB
- New costbasis advertises second granularity and sticks to it

Our options, as I see them, are:

1. The easiest thing to do long term will be to write the lines exactly the same as qbert does, so we can compare side-by-side on all things. That means we need to convince costbasis to round up those timestamps.

2. The second easiest thing long term is to bump the timestamp resolution on those database fields, and change the volume fields to be `cal.MillisTimestamp` instead of `cal.SqlTimestamp`. Then when we compare lines to qbert, we remember to round. Something like this:

    ```sql
    SELECT CAST(time_millis AS DATETIME) FROM test_table;
    ```

    This is arguably the most correct thing, since we're not losing data. And, it might be more straightforward than messing with (and testing) the rounding, since it's not obvious what other systems that would break.

3. The easiest thing for right now (but maybe problems down the line) is to just not worry about it.

### Of those, I recommend option 2 (increase the timestamp resolution on volumes)

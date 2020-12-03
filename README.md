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

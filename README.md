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

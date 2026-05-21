# Option Replay Data Pipeline

## Overview

This pipeline processes historical options minute aggregate data downloaded from Massive.com and converts it into optimized parquet files for use by the option replay engine.

The pipeline is designed with the following goals:

- Fast ingestion
- Immutable finalized expiry files
- Safe restart/recovery
- Efficient parquet storage
- Predictable file organization
- Minimal memory usage
- Efficient replay lookups

---

# Data Source

Provider:
- Massive.com

Dataset:
- Options minute aggregates

Raw source format:
- Daily `.csv.gz` files

Example:

/f/data/minute_aggs_v1/2026/05/2026-05-11.csv.gz

---

# Raw Source Schema

```csv
ticker,volume,open,close,high,low,window_start,transactions
O:SPY230327P00390000,1,11.82,11.82,11.82,11.82,1678715580000000000,1
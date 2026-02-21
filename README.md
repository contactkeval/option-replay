# Option-Replay
### High-performance strategy testing, made simpler.
**Define. Replay. Refine.**

[![Go Report Card](https://goreportcard.com/badge/github.com/contactkeval/option-replay)](https://goreportcard.com/report/github.com/contactkeval/option-replay)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A high-performance, modular options backtesting engine written in Go. `option-replay` is designed to simulate complex options strategies using historical minute-level data, featuring an intelligent local data caching system to minimize API latency and costs.

## ?? Key Features

* **Symmetric Architecture**: Clean separation between the **Sequencer** (time management), **Planner** (strategy logic), and **Executor** (trade execution).
* **Intelligent Data Hydration**: Automatically manages local CSV storage. If data is missing, it fetches from a secondary provider and appends it to local storage.
* **OCC Symbol Support**: Native parsing and generation of OCC-style option symbols (e.g., `O:SPY250131C00450000`).
* **Automated Maintenance**: A built-in pipeline to keep your "blue-chip" symbols and active option chains updated with the latest market data.
* **Performance Focused**: Leverages Go's efficiency for rapid iteration over multi-year datasets.

---

## ?? System Architecture

The project follows a "Sequence ? Plan ? Execute" flow to ensure chronological integrity during backtests:

1.  **Sequencer**: Iterates through the timeline (MinuteRow by MinuteRow).
2.  **Planner**: Interprets market data and decides on entries, exits, or adjustments.
3.  **Executor**: Manages the lifecycle of active trades, including slippage simulation and fill logic.



---

## ?? Data Management

The engine uses a `LocalCSVProvider` to store market data locally. A `manifest.csv` acts as a catalog to track the date ranges available for each symbol.

### Data Requirements Logic:
- **Stocks/Indices**: Ensures data is available for the requested range; fills gaps older than 2 years or up to the current date.
- **Options**: Automatically fetches data starting **2 years prior to the expiration date** through today to ensure full historical context for Greeks and volatility calculations.

---

## ?? Installation & Usage

### Prerequisites
- Go 1.21 or higher
- API Key for a secondary data provider (e.g., Polygon.io)

### Setup
```bash
# Clone the repository
git clone [https://github.com/contactkeval/option-replay.git](https://github.com/contactkeval/option-replay.git)

# Install dependencies
cd option-replay
go mod tidy
```

## ⚙️ Configuration Guide

The engine is driven by a `.json` configuration file. This allows you to define complex strategies (like Iron Condors, Diagonals, or Covered Calls) without recompiling the code.

### Example: Double Diagonal / Complex Spread
```json
{
  "underlying": "AAPL",
  "entry": {
    "start": "2026-01-01",
    "end": "2026-01-31",
    "mode": "daily_time",
    "date_match_type": "higher",
    "time_of_day": "9:45",
    "timezone": "America/New_York"
  },
  "exit": {
    "exit_by_days_to_expiry": 0,
    "max_days_in_trade": 1
  },
  "strategy": {
    "name": "double_diagonal",
    "legs": [
      { "side": "sell", "option_type": "call", "strike_rule": "ATM", "qty": 1, "expiration": 2 },
      { "side": "sell", "option_type": "put",  "strike_rule": "ATM", "qty": 1, "expiration": 2 },
      { "side": "buy",  "option_type": "call", "strike_rule": "ATM", "qty": 1, "expiration": 9 },
      { "side": "buy",  "option_type": "put",  "strike_rule": "ATM", "qty": 1, "expiration": 9 }
    ]
  },
  "data_provider": { "provider": "massive" }
}

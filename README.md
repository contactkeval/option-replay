# option-replay

Modular rewrite of options strategy backtester.

Run (CLI):

```bash
export POLYGON_API_KEY=your_key_here
go run ./cmd/option-replay -config config.json
```

Run as REST server:

```bash
go run ./cmd/option-replay -config config.json -rest -port :8080
curl http://localhost:8080/run
```

Outputs written to `output_dir` specified in config (default `./out`): `trades.json` and `trades.csv`.

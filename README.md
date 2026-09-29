# The Last Call

A speakeasy with golf simulators. This repo is the site and the bay booking
system behind it.

> The fairway never closes. The bar does.

## Stack

| Layer    | Choice                                                    |
| -------- | --------------------------------------------------------- |
| Backend  | Go 1.25, standard-library `net/http` routing, no framework |
| Frontend | React 19 + TypeScript, built with Vite                     |
| Storage  | DynamoDB, single table (in-memory store for dev and tests) |
| Deploy   | One binary — the built frontend is embedded with `go:embed`|

### Why DynamoDB holds up here

Double-booking a bay is the only real failure mode in this system, and the
table layout makes it impossible rather than unlikely. Every bay-hour is its
own item:

```
reservation    PK = RES#<id>       SK = RES#<id>
bay-hour hold  PK = NIGHT#<date>   SK = HOUR#<hh>#BAY#<n>
```

A hold writes the reservation and one item per hour it covers inside a single
`TransactWriteItems`, each under `attribute_not_exists`. Either the whole
stretch is ours or nothing is written — no locks, no read-modify-write window.
Availability for a night is one query against its partition.

## Running it

```sh
make deps            # install frontend dependencies, once
make dev             # API on :8080 + Vite on :5173, hot reload
```

Against real DynamoDB semantics:

```sh
make dynamo          # DynamoDB Local on :8042
make run-dynamo
```

A production-shaped build — one binary serving the API and the site:

```sh
make build && ./bin/thelastcall
```

```sh
make check           # go vet, go test -race, gofmt, tsc
```

## Configuration

Every setting is an environment variable with a house default.

| Variable                  | Default                | Meaning                                    |
| ------------------------- | ---------------------- | ------------------------------------------ |
| `TLC_ADDR`                | `:8080`                | Listen address                             |
| `TLC_STORE`               | `memory`               | `memory` or `dynamo`                       |
| `TLC_DYNAMO_TABLE`        | `thelastcall`          | Table name                                 |
| `TLC_DYNAMO_ENDPOINT`     | —                      | Set to point at DynamoDB Local             |
| `TLC_DYNAMO_AUTOCREATE`   | `false`                | Create the table on boot (local dev only)  |
| `TLC_BAYS`                | `3`                    | How many sim bays                          |
| `TLC_MAX_PARTY`           | `6`                    | Largest party one bay seats                |
| `TLC_BOOKING_WINDOW_DAYS` | `7`                    | How far ahead guests may book              |
| `TLC_BOOKING_LEAD`        | `30m`                  | Notice the bar needs before a tee time     |
| `TLC_TIMEZONE`            | `America/Los_Angeles`  | The house clock                            |
| `TLC_ALLOWED_ORIGINS`     | —                      | Comma-separated CORS origins (dev)         |

Hours live in `internal/booking/hours.go` as `DefaultSchedule`. They are the
single source of truth: the booking rules enforce them and `/api/v1/house`
publishes them, so the copy on the site can never drift from what the server
will actually accept.

## API

| Method   | Path                      | Purpose                                 |
| -------- | ------------------------- | --------------------------------------- |
| `GET`    | `/api/v1/health`          | Liveness probe                          |
| `GET`    | `/api/v1/house`           | Hours, bay count, party limit           |
| `GET`    | `/api/v1/availability`    | Seven-night strip (`?party=&hours=`)    |
| `POST`   | `/api/v1/bookings`        | Hold a bay → `201` with a code          |
| `GET`    | `/api/v1/bookings/{id}`   | Read a reservation                      |
| `DELETE` | `/api/v1/bookings/{id}`   | Release a reservation                   |

A full bay returns `409`; a request outside the night's window returns `422`.

## Layout

```
cmd/server/          entry point, wiring, graceful shutdown
internal/booking/    the rulebook: hours, availability, holds
internal/store/      DynamoDB and in-memory backends
internal/httpapi/    routing, handlers, middleware, static serving
internal/webui/      go:embed of the built frontend
web/                 React + TypeScript source
```

## Screens

The design lives in Claude Design. `Book a Bay` is built; `The Last Call`,
`Hours`, `Find Us` and `Knock` are still placeholders that keep the nav honest.

Hover states in the design source are `style-hover` annotations, which the
design runtime ignores. Here they are real `:hover` rules in
`web/src/styles.css`, where the palette and type scale also live as tokens.

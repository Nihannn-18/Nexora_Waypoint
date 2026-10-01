// The module path is deliberately not a GitHub URL. This service is never
// imported from outside the monorepo, and a domain-style path means the module
// never has to be renamed if the repository moves or the owner changes.
//
// Imports look like: waypoint.lk/api/internal/planning
module waypoint.lk/api

go 1.24

// No third-party dependencies yet — the scaffold builds with the standard
// library alone so `nx build api` is green on a fresh clone with no network.
// Add what you need (router, pgx, amqp091-go, jwt) as you go; `go mod tidy`
// will populate require() and go.sum.

package db

import (
    "context"
    "log"

    "github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Init(url string) {
    var err error
    Pool, err = pgxpool.New(context.Background(), url)
    if err != nil {
        log.Fatalf("Unable to connect to database: %v\n", err)
    }
}

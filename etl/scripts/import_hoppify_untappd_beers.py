from __future__ import annotations

import argparse
import glob
import os
import time
from pathlib import Path
from typing import Any

import duckdb
import psycopg
from tqdm import tqdm


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_PARQUET = REPO_ROOT / "apps" / "hoppify" / "data" / "beers.parquet"

UNTAPPD_BEER_COLUMNS = (
    "untappd_id",
    "url",
    "untappd_slug",
    "brewery_prefix",
    "search_text",
    "last_modified_at",
)

UNTAPPD_BEER_INDEXES = (
    """
CREATE INDEX IF NOT EXISTS untappd_beers_slug_untappd_id_idx
    ON untappd_beers (untappd_slug, untappd_id)
""",
    """
CREATE INDEX IF NOT EXISTS untappd_beers_brewery_prefix_idx
    ON untappd_beers (brewery_prefix)
    WHERE brewery_prefix IS NOT NULL
""",
    """
CREATE INDEX IF NOT EXISTS untappd_beers_last_modified_at_idx
    ON untappd_beers (last_modified_at DESC, untappd_id DESC)
""",
    """
CREATE INDEX IF NOT EXISTS untappd_beers_search_text_fts_idx
    ON untappd_beers USING gin (to_tsvector('simple', search_text))
""",
    """
CREATE INDEX IF NOT EXISTS untappd_beers_search_text_trgm_idx
    ON untappd_beers USING gin (search_text gin_trgm_ops)
""",
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Import Hoppify Untappd beer parquet catalog into Postgres."
    )
    parser.add_argument(
        "--parquet",
        default=str(DEFAULT_PARQUET),
        help=f"Input beers parquet file, default: {DEFAULT_PARQUET}",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=100_000,
        help="Rows to read from DuckDB and COPY per batch.",
    )
    parser.add_argument(
        "--limit",
        type=int,
        help="Optional source row limit for smoke tests.",
    )
    parser.add_argument(
        "--truncate",
        action="store_true",
        help="TRUNCATE untappd_beers before importing.",
    )
    parser.add_argument(
        "--rebuild-indexes",
        action="store_true",
        help="Drop secondary untappd_beers indexes before COPY and recreate them afterwards.",
    )
    parser.add_argument(
        "--no-progress",
        action="store_true",
        help="Do not show a progress bar.",
    )
    parser.add_argument(
        "--skip-analyze",
        action="store_true",
        help="Do not run ANALYZE untappd_beers after import.",
    )
    parser.add_argument(
        "--dsn",
        help="Postgres DSN. Defaults to DATABASE_URL or POSTGRES_* env vars.",
    )
    return parser.parse_args()


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def connect_postgres(args: argparse.Namespace) -> psycopg.Connection[Any]:
    dsn = args.dsn or os.getenv("DATABASE_URL")
    if dsn:
        return psycopg.connect(dsn)

    return psycopg.connect(
        dbname=os.getenv("POSTGRES_DB", "hoppify"),
        user=os.getenv("POSTGRES_USER", "hoppify"),
        password=os.getenv("POSTGRES_PASSWORD", "hoppify"),
        host=os.getenv("POSTGRES_HOST", "127.0.0.1"),
        port=os.getenv("POSTGRES_PORT", "5432"),
    )


def execute(conn: psycopg.Connection[Any], sql: str) -> None:
    with conn.cursor() as cur:
        cur.execute(sql)
    conn.commit()


def check_target_table(conn: psycopg.Connection[Any]) -> None:
    with conn.cursor() as cur:
        cur.execute("SELECT to_regclass('public.untappd_beers')")
        if cur.fetchone()[0] is None:
            raise RuntimeError(
                "public.untappd_beers does not exist; run apps/hoppify/migrations first"
            )


def validate_source(duck: duckdb.DuckDBPyConnection, parquet: str) -> None:
    validation = duck.execute(
        f"""
SELECT
    count(*) AS row_count,
    count(*) FILTER (
        WHERE url IS NULL
            OR untappd_id IS NULL
            OR slog IS NULL
            OR last_modified_at IS NULL
            OR trim(slog) = ''
    ) AS invalid_required,
    count(*) FILTER (
        WHERE regexp_extract(url, '/b/([^/]+)/[0-9]+$', 1) <> slog
    ) AS url_slug_mismatch,
    count(*) FILTER (
        WHERE try_cast(regexp_extract(url, '/([0-9]+)$', 1) AS UBIGINT) <> untappd_id
    ) AS url_id_mismatch,
    count(DISTINCT untappd_id) AS distinct_untappd_id,
    count(DISTINCT url) AS distinct_url
FROM read_parquet({sql_literal(parquet)})
"""
    ).fetchone()
    if validation is None:
        raise RuntimeError("source validation returned no rows")

    (
        row_count,
        invalid_required,
        url_slug_mismatch,
        url_id_mismatch,
        distinct_untappd_id,
        distinct_url,
    ) = validation

    if row_count == 0:
        raise RuntimeError("source parquet is empty")
    if invalid_required:
        raise RuntimeError(f"source parquet has invalid required values: {invalid_required}")
    if url_slug_mismatch:
        raise RuntimeError(f"source parquet has url/slog mismatches: {url_slug_mismatch}")
    if url_id_mismatch:
        raise RuntimeError(f"source parquet has url/untappd_id mismatches: {url_id_mismatch}")
    if distinct_untappd_id != row_count:
        raise RuntimeError(
            f"source parquet has duplicate untappd_id values: rows={row_count} distinct={distinct_untappd_id}"
        )
    if distinct_url != row_count:
        raise RuntimeError(
            f"source parquet has duplicate url values: rows={row_count} distinct={distinct_url}"
        )


def source_count_sql(parquet: str, limit: int | None) -> str:
    return f"SELECT count(*) FROM ({source_sql(parquet, limit)}) AS src"


def source_sql(parquet: str, limit: int | None) -> str:
    row_limit = f"\nLIMIT {limit}" if limit is not None else ""

    return f"""
SELECT
    CAST(untappd_id AS BIGINT) AS untappd_id,
    url,
    slog AS untappd_slug,
    brewery_prefix,
    trim(regexp_replace(lower(slog), '[^a-z0-9]+', ' ', 'g')) AS search_text,
    CAST(last_modified_at AS VARCHAR) AS last_modified_at
FROM read_parquet({sql_literal(parquet)})
ORDER BY untappd_id
{row_limit}
"""


def drop_indexes(conn: psycopg.Connection[Any]) -> None:
    index_names = (
        "untappd_beers_slug_untappd_id_idx",
        "untappd_beers_brewery_prefix_idx",
        "untappd_beers_last_modified_at_idx",
        "untappd_beers_search_text_fts_idx",
        "untappd_beers_search_text_trgm_idx",
    )
    for index_name in index_names:
        execute(conn, f"DROP INDEX IF EXISTS {index_name}")


def create_indexes(conn: psycopg.Connection[Any]) -> None:
    for index_sql in UNTAPPD_BEER_INDEXES:
        execute(conn, index_sql)


def copy_untappd_beers(
    conn: psycopg.Connection[Any],
    rows: list[tuple[Any, ...]],
) -> None:
    columns = ", ".join(UNTAPPD_BEER_COLUMNS)
    with conn.cursor() as cur:
        with cur.copy(f"COPY untappd_beers ({columns}) FROM STDIN") as copy:
            for row in rows:
                copy.write_row(row)
    conn.commit()


def main() -> None:
    args = parse_args()
    if args.batch_size <= 0:
        raise ValueError("--batch-size must be positive")
    if args.limit is not None and args.limit <= 0:
        raise ValueError("--limit must be positive")
    if not glob.glob(args.parquet):
        raise FileNotFoundError(args.parquet)

    started = time.monotonic()
    duck = duckdb.connect()
    validate_source(duck, args.parquet)

    with connect_postgres(args) as pg:
        check_target_table(pg)
        execute(pg, "SET synchronous_commit = off")

        total = None
        if not args.no_progress:
            total = duck.execute(source_count_sql(args.parquet, args.limit)).fetchone()[0]

        if args.rebuild_indexes:
            drop_indexes(pg)

        if args.truncate:
            execute(pg, "TRUNCATE TABLE untappd_beers")

        cursor = duck.execute(source_sql(args.parquet, args.limit))
        seen = 0
        inserted = 0
        batch_no = 0

        progress = tqdm(
            total=total,
            unit="rows",
            disable=args.no_progress,
            dynamic_ncols=True,
        )
        with progress:
            while True:
                batch = cursor.fetchmany(args.batch_size)
                if not batch:
                    break

                batch_no += 1
                seen += len(batch)
                copy_untappd_beers(pg, batch)
                inserted += len(batch)

                progress.update(len(batch))
                progress.set_postfix(batch=batch_no, inserted=inserted)

                if args.no_progress:
                    elapsed = max(time.monotonic() - started, 0.001)
                    rate = inserted / elapsed
                    print(
                        "batch={batch_no} seen={seen} inserted={inserted} rate={rate:.0f}/s".format(
                            batch_no=batch_no,
                            seen=seen,
                            inserted=inserted,
                            rate=rate,
                        ),
                        flush=True,
                    )

        if args.rebuild_indexes:
            create_indexes(pg)

        if not args.skip_analyze:
            execute(pg, "ANALYZE untappd_beers")

    elapsed = time.monotonic() - started
    print(
        "done seen={seen} inserted={inserted} elapsed={elapsed:.1f}s".format(
            seen=seen,
            inserted=inserted,
            elapsed=elapsed,
        ),
        flush=True,
    )


if __name__ == "__main__":
    main()

from __future__ import annotations

import argparse
import glob
import json
import os
import time
from typing import Any

import duckdb
import psycopg
from tqdm import tqdm


DEFAULT_OVERTURE_RELEASE = "2026-05-20.0"
DEFAULT_SUBTYPES = ("country", "region", "locality")

GEO_FEATURE_COLUMNS = (
    "overture_release",
    "overture_area_id",
    "overture_division_id",
    "subtype",
    "class",
    "country",
    "region",
    "primary_name",
    "names_text",
    "names_json",
    "wikidata",
    "population",
    "parent_overture_division_id",
    "admin_level",
    "local_type",
    "is_land",
    "is_territorial",
    "west_lon",
    "south_lat",
    "east_lon",
    "north_lat",
    "overture_sources",
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Import Overture division bbox records into Postgres geo_features."
    )
    parser.add_argument(
        "--release",
        default=DEFAULT_OVERTURE_RELEASE,
        help=f"Overture release to import, default: {DEFAULT_OVERTURE_RELEASE}",
    )
    parser.add_argument(
        "--division-area-parquet-glob",
        help="Input division_area parquet glob. Defaults to Overture public S3 for --release.",
    )
    parser.add_argument(
        "--division-parquet-glob",
        help="Input division parquet glob. Defaults to Overture public S3 for --release.",
    )
    parser.add_argument(
        "--subtypes",
        default=",".join(DEFAULT_SUBTYPES),
        help="Comma-separated division_area subtypes to import.",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=25_000,
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
        help="TRUNCATE geo_features before importing.",
    )
    parser.add_argument(
        "--no-progress",
        action="store_true",
        help="Do not show a progress bar.",
    )
    parser.add_argument(
        "--skip-analyze",
        action="store_true",
        help="Do not run ANALYZE geo_features after import.",
    )
    parser.add_argument(
        "--dsn",
        help="Postgres DSN. Defaults to DATABASE_URL or POSTGRES_* env vars.",
    )
    return parser.parse_args()


def default_overture_glob(release: str, overture_type: str) -> str:
    return (
        "s3://overturemaps-us-west-2/release/"
        f"{release}/theme=divisions/type={overture_type}/*"
    )


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def sql_string_list(values: tuple[str, ...]) -> str:
    return ", ".join(sql_literal(value) for value in values)


def is_remote_path(path: str) -> bool:
    return path.startswith(("s3://", "http://", "https://"))


def connect_postgres(args: argparse.Namespace) -> psycopg.Connection[Any]:
    dsn = args.dsn or os.getenv("DATABASE_URL")
    if dsn:
        return psycopg.connect(dsn)

    return psycopg.connect(
        dbname=os.getenv("POSTGRES_DB", "fsqr"),
        user=os.getenv("POSTGRES_USER", "fsqr"),
        password=os.getenv("POSTGRES_PASSWORD", "fsqr"),
        host=os.getenv("POSTGRES_HOST", "127.0.0.1"),
        port=os.getenv("POSTGRES_PORT", "5432"),
    )


def configure_duckdb(duck: duckdb.DuckDBPyConnection, paths: tuple[str, ...]) -> None:
    if any(path.startswith("s3://") for path in paths):
        duck.execute("INSTALL httpfs")
        duck.execute("LOAD httpfs")
        duck.execute("SET s3_region='us-west-2'")


def parse_subtypes(raw: str) -> tuple[str, ...]:
    subtypes = tuple(part.strip() for part in raw.split(",") if part.strip())
    if not subtypes:
        raise ValueError("--subtypes must contain at least one subtype")
    return subtypes


def source_sql(
    division_area_glob: str,
    division_glob: str,
    subtypes: tuple[str, ...],
    limit: int | None,
) -> str:
    row_limit = f"\nLIMIT {limit}" if limit is not None else ""

    return f"""
SELECT
    a.id AS overture_area_id,
    a.division_id AS overture_division_id,
    COALESCE(d.subtype, a.subtype) AS subtype,
    COALESCE(d.class, a.class) AS class,
    COALESCE(d.country, a.country) AS country,
    COALESCE(d.region, a.region) AS region,
    a.names AS area_names,
    d.names AS division_names,
    d.wikidata,
    d.population,
    d.parent_division_id,
    d.admin_level,
    d.local_type,
    a.is_land,
    a.is_territorial,
    struct_extract(a.bbox, 'xmin') AS west_lon,
    struct_extract(a.bbox, 'ymin') AS south_lat,
    struct_extract(a.bbox, 'xmax') AS east_lon,
    struct_extract(a.bbox, 'ymax') AS north_lat,
    a.sources
FROM read_parquet(
    {sql_literal(division_area_glob)},
    hive_partitioning = true,
    union_by_name = true
) AS a
LEFT JOIN read_parquet(
    {sql_literal(division_glob)},
    hive_partitioning = true,
    union_by_name = true
) AS d
    ON d.id = a.division_id
WHERE a.subtype IN ({sql_string_list(subtypes)})
    AND a.class = 'land'
    AND a.is_land = true
    AND COALESCE(d.country, a.country) IS NOT NULL
    AND struct_extract(a.bbox, 'xmin') IS NOT NULL
    AND struct_extract(a.bbox, 'ymin') IS NOT NULL
    AND struct_extract(a.bbox, 'xmax') IS NOT NULL
    AND struct_extract(a.bbox, 'ymax') IS NOT NULL
    AND struct_extract(a.bbox, 'xmin') BETWEEN -180 AND 180
    AND struct_extract(a.bbox, 'xmax') BETWEEN -180 AND 180
    AND struct_extract(a.bbox, 'ymin') BETWEEN -90 AND 90
    AND struct_extract(a.bbox, 'ymax') BETWEEN -90 AND 90
    AND struct_extract(a.bbox, 'xmin') <= struct_extract(a.bbox, 'xmax')
    AND struct_extract(a.bbox, 'ymin') <= struct_extract(a.bbox, 'ymax')
{row_limit}
"""


def check_inputs(paths: tuple[str, ...]) -> None:
    for path in paths:
        if not is_remote_path(path) and not glob.glob(path):
            raise FileNotFoundError(path)


def check_target_table(conn: psycopg.Connection[Any]) -> None:
    with conn.cursor() as cur:
        cur.execute("SELECT to_regclass('public.geo_features')")
        if cur.fetchone()[0] is None:
            raise RuntimeError(
                "public.geo_features does not exist; run apps/fsqr/migrations/004_create_geo_features.sql first"
            )


def execute(conn: psycopg.Connection[Any], sql: str) -> None:
    with conn.cursor() as cur:
        cur.execute(sql)
    conn.commit()


def normalize_json_value(value: Any) -> Any:
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if isinstance(value, dict):
        return {
            str(key): normalize_json_value(item)
            for key, item in value.items()
            if item is not None
        }
    if isinstance(value, (list, tuple)):
        return [normalize_json_value(item) for item in value]
    return str(value)


def json_text(value: Any, fallback: str) -> str:
    normalized = normalize_json_value(value)
    if normalized is None:
        return fallback
    return json.dumps(normalized, ensure_ascii=False, separators=(",", ":"))


def add_name(values: list[str], seen: set[str], value: Any) -> None:
    if not isinstance(value, str):
        return

    name = " ".join(value.split())
    if name and name not in seen:
        values.append(name)
        seen.add(name)


def add_name_values(values: list[str], seen: set[str], raw: Any) -> None:
    if isinstance(raw, str):
        add_name(values, seen, raw)
        return
    if isinstance(raw, (list, tuple)):
        for item in raw:
            add_name_values(values, seen, item)


def append_common_names(values: list[str], seen: set[str], common: Any) -> None:
    if not isinstance(common, dict):
        return

    if isinstance(common.get("value"), (list, tuple)):
        add_name_values(values, seen, common["value"])
        return

    for name in common.values():
        add_name_values(values, seen, name)


def append_rule_names(values: list[str], seen: set[str], rules: Any) -> None:
    if not isinstance(rules, (list, tuple)):
        return

    for rule in rules:
        if isinstance(rule, dict):
            add_name(values, seen, rule.get("value"))


def flatten_names(names: Any) -> list[str]:
    values: list[str] = []
    seen: set[str] = set()

    if isinstance(names, dict):
        add_name(values, seen, names.get("primary"))
        append_common_names(values, seen, names.get("common"))
        append_rule_names(values, seen, names.get("rules"))
    else:
        add_name(values, seen, names)

    return values


def prepared_row(
    release: str,
    row: tuple[Any, ...],
) -> tuple[Any, ...] | None:
    (
        overture_area_id,
        overture_division_id,
        subtype,
        geo_class,
        country,
        region,
        area_names,
        division_names,
        wikidata,
        population,
        parent_division_id,
        admin_level,
        local_type,
        is_land,
        is_territorial,
        west_lon,
        south_lat,
        east_lon,
        north_lat,
        sources,
    ) = row

    names = division_names or area_names
    flattened_names = flatten_names(names)
    if not flattened_names:
        return None

    return (
        release,
        overture_area_id,
        overture_division_id,
        subtype,
        geo_class,
        country,
        region,
        flattened_names[0],
        " ".join(flattened_names),
        json_text(names, "{}"),
        wikidata,
        population,
        parent_division_id,
        admin_level,
        local_type,
        is_land,
        is_territorial,
        west_lon,
        south_lat,
        east_lon,
        north_lat,
        json_text(sources, "[]"),
    )


def copy_geo_features(
    conn: psycopg.Connection[Any],
    rows: list[tuple[Any, ...]],
) -> None:
    columns = ", ".join(GEO_FEATURE_COLUMNS)
    with conn.cursor() as cur:
        with cur.copy(f"COPY geo_features ({columns}) FROM STDIN") as copy:
            for row in rows:
                copy.write_row(row)
    conn.commit()


def main() -> None:
    args = parse_args()
    if args.batch_size <= 0:
        raise ValueError("--batch-size must be positive")
    if args.limit is not None and args.limit <= 0:
        raise ValueError("--limit must be positive")

    subtypes = parse_subtypes(args.subtypes)
    division_area_glob = args.division_area_parquet_glob or default_overture_glob(
        args.release,
        "division_area",
    )
    division_glob = args.division_parquet_glob or default_overture_glob(
        args.release,
        "division",
    )
    input_paths = (division_area_glob, division_glob)
    check_inputs(input_paths)

    started = time.monotonic()
    duck = duckdb.connect()
    configure_duckdb(duck, input_paths)

    with connect_postgres(args) as pg:
        check_target_table(pg)
        execute(pg, "SET synchronous_commit = off")

        if args.truncate:
            execute(pg, "TRUNCATE TABLE geo_features RESTART IDENTITY")

        cursor = duck.execute(
            source_sql(division_area_glob, division_glob, subtypes, args.limit)
        )
        seen = 0
        inserted = 0
        skipped_without_names = 0
        batch_no = 0

        progress = tqdm(
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
                prepared = []
                for row in batch:
                    item = prepared_row(args.release, row)
                    if item is None:
                        skipped_without_names += 1
                    else:
                        prepared.append(item)

                if prepared:
                    copy_geo_features(pg, prepared)
                    inserted += len(prepared)

                progress.update(len(batch))
                progress.set_postfix(
                    batch=batch_no,
                    inserted=inserted,
                    skipped_without_names=skipped_without_names,
                )

                if args.no_progress:
                    elapsed = max(time.monotonic() - started, 0.001)
                    rate = inserted / elapsed
                    print(
                        "batch={batch_no} seen={seen} inserted={inserted} "
                        "skipped_without_names={skipped_without_names} rate={rate:.0f}/s".format(
                            batch_no=batch_no,
                            seen=seen,
                            inserted=inserted,
                            skipped_without_names=skipped_without_names,
                            rate=rate,
                        ),
                        flush=True,
                    )

        if not args.skip_analyze:
            execute(pg, "ANALYZE geo_features")

    elapsed = time.monotonic() - started
    print(
        "done seen={seen} inserted={inserted} skipped_without_names={skipped_without_names} "
        "elapsed={elapsed:.1f}s".format(
            seen=seen,
            inserted=inserted,
            skipped_without_names=skipped_without_names,
            elapsed=elapsed,
        ),
        flush=True,
    )


if __name__ == "__main__":
    main()

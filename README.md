# fsqr

[![powered by PostgreSQL](https://img.shields.io/badge/powered%20by-PostgreSQL-336791?style=flat-square)](https://www.postgresql.org/)
[![data Foursquare OS Places](https://img.shields.io/badge/data-Foursquare%20OS%20Places-333333?style=flat-square)](https://opensource.foursquare.com/os-places/)
[![data Overture Maps](https://img.shields.io/badge/data-Overture%20Maps-2563eb?style=flat-square)](https://overturemaps.org/)
[![hosted by Hetzner](https://img.shields.io/badge/hosted%20by-Hetzner-d50c2d?style=flat-square)](https://www.hetzner.com/)
[![maps by Mapbox](https://img.shields.io/badge/maps%20by-Mapbox-000000?style=flat-square)](https://www.mapbox.com/)

fsqr is a fast semantic geosearch API and map UI for place discovery. It combines semantic category matching with geographic filtering to return compact, map-ready place results.

## Stack

- Monorepo layout with applications under `apps/`.
- `fsqr` Go service rooted at `apps/fsqr`.
- `hoppify` Go service rooted at `apps/hoppify`.
- PostgreSQL-backed search and place storage.
- Foursquare OS Places as the POI source.
- Overture Maps divisions as the planned geographic boundary/source layer.
- Mapbox tiles for the hosted map UI when configured.
- Leaflet web UI served from `apps/fsqr/internal/http/web`.
- Shared gateway, deployment, and observability assets live in `gateway/`, `deployment/`, and `observability/`.

## Development

Use `just` as the main task runner:

```sh
just service-build
just lint
cd apps/fsqr && go test ./...
cd apps/hoppify && go test ./...
```

Run the e2e stack:

```sh
just up
just test-e2e
just down
```

## Data Attribution

Place data is based on [Foursquare OS Places](https://opensource.foursquare.com/os-places/).

Geographic boundary exploration uses [Overture Maps](https://overturemaps.org/) divisions data. Overture Divisions includes OpenStreetMap-derived data and is available under ODbL; public use should include appropriate attribution such as:

```text
© OpenStreetMap contributors, Overture Maps Foundation
```

See the [Overture attribution and licensing page](https://docs.overturemaps.org/attribution/) for source-specific requirements.

Hosted map tiles are provided by [Mapbox](https://www.mapbox.com/) when `FSQR_WEB_MAPBOX_ACCESS_TOKEN` is configured.

---
id: 0009-images-in-database
title: "0009: Uploaded images are stored in the database"
---

**Status:** accepted, 2026-10-14

## Context

Menu photos were files under `<data_dir>/assets/`, with a row in `assets` pointing at them. That works for one process on one disk. On a platform that runs several instances with throwaway disks (the Vercel demo), the row was shared through the database but the file lived on one instance only: photos broke on other instances and disappeared on every restart. Backups also had to cover two things, the database and the directory.

## Decision

* The image bytes are a column of the `assets` row (`BLOB` on SQLite, `bytea` on PostgreSQL). Every instance serves them from the database.
* Each image is capped by `uploads.max_image_bytes`, 1 MB by default (64 KB to 20 MB). The web app shrinks larger photos in the browser (longest side up to 2048 px, JPEG) before uploading, so a phone photo still fits.
* At start, servly copies images that older versions left on disk into the database, once. Missing files are logged and skipped.
* Public images are served with `Cache-Control: public, max-age=86400, immutable`. An image never changes; a new photo is a new asset.

## Alternatives

* **Object storage (S3, Vercel Blob):** suits large media libraries, but needs an external service and credentials, and self-hosted installs would still need the disk path, so two storage paths to maintain.
* **Keep files and require a shared volume:** platforms like Vercel have none.

## Consequences

* The database is the whole backup. An image adds at most the limit to it: 100 photos at 1 MB stay under Neon's free 0.5 GB.
* Serving an image reads its bytes from the database. Browsers and proxies cache them, so each guest fetches a photo once.
* Images uploaded to an instance's disk before this change are lost unless that disk still exists at the first start of the new version.

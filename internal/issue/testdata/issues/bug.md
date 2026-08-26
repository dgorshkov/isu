---
schema: 1
id: ISU-7f3akq
title: Login retries stop after the third attempt
type: bug
state: open
owner: dmitry
created: 2026-08-24
priority: p1
repro: POST /session five times with a bad password, watch the fourth 500
---
The retry counter is reset by the wrong branch of the backoff, so the fourth
attempt reads an uninitialised deadline.

## What good looks like

Five attempts, then a 429.

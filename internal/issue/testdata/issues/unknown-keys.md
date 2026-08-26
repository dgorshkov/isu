---
schema: 1
id: PROJ-1234
title: Imported from Jira
type: bug
state: open
owner: dmitry
created: 2026-07-14
repro: See the attached HAR
jira_key: PROJ-1234
jira_reporter: someone@example.com
imported_at: 2026-08-24T09:12:00Z
---
Unknown keys survive a round trip untouched, which is what lets an importer
carry provenance without every reader having to know about it.

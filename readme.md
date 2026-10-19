# Haven Cash Cannon

A small Go service that pays Haven events their signup autogrants through the
[HCB v4 API](https://hcb.hackclub.com/docs/api/v4), driven by the Haven Airtable
base. Adapted from the Campfire cash cannon (`main` branch); this is the `haven`
branch.

## How it works

1. `/` (HTTP basic auth) shows a dashboard with two actions: **Autogrant** and
   **Custom miscellaneous** disbursements. Both open a preview first.
2. The preview lists every eligible event (`Status = Active`, not a test event,
   has an `HCB ID`) with its Entitled / Disbursed / Owed amounts, and reads the
   live balance of the paying HCB org. The confirm button is disabled if the
   balance cannot cover the grants.
3. On confirm, for each event with a non-zero amount the app:
   - creates a row in the Airtable **Disbursements** table (`Status = Pending`,
     `Type = Autogrant` / `Withdrawal` / `Miscellaneous`, full context in `Notes`),
   - POSTs `/api/v4/organizations/<source>/transfers` on HCB
     (`to_organization_id` = the event's `HCB ID`, memo `Haven signup grant #<id>`),
   - marks the row `Processed` (or `Failed` with the error) and appends the HCB
     response to `Notes`.
4. The Events table's `Disbursed Autogrant Amount` rollup sums only
   `Processed` + `Autogrant` rows, so `Owed Amount` drops to zero after a
   successful run and a second click pays nothing. Failed, Pending and
   Miscellaneous rows never reduce what is owed.

Negative `Owed Amount` (an event over-paid, e.g. attendees withdrew) is pulled
back as a withdrawal from the event org to the source org.

Safety: one run at a time (second request gets HTTP 409), amounts are rounded
to whole cents, a run is refused up front when HCB funds are insufficient, and
every Airtable field is addressed by field ID so renames cannot break payouts.

## Airtable (base `appc9YSRmmAmLI37u`)

| Table | Field | Used for |
|---|---|---|
| Events `tbl0BQWphHe0gWEIp` | HCB ID `fldsW0bQnkXwQVzZw` | destination org (`org_…`) |
| | Owed Amount `fldEWH9DlsmNkgP6Z` | amount to pay |
| | Entitled / Disbursed Autogrant Amount | shown in preview |
| Disbursements `tblnDFHiIEvVPWcqH` | ID, Event, Amount, Status, Type, Notes | audit trail |

## Configuration

See `.env.example`. Required: `AIRTABLE_API_KEY`, `HCB_API_TOKEN`
(`transfers:write` scope), `BASIC_AUTH_USERNAME`, `BASIC_AUTH_PASSWORD`.
Everything else has Haven defaults.

## Run

```sh
cp .env.example .env   # fill in
go run .               # or: docker build -t cash-cannon . && docker run --env-file .env -p 8080:8080 cash-cannon
```

`GET /healthz` is unauthenticated for container health checks.

# Native metric table tests

From `internal/site`, install dependencies following the project's contributing guide. Use a supported Node.js release (Vite requires Node.js 20.19+ or 22.12+) and make Bun available for the focused tests:

```sh
bun run build
bun test tests/chart-accessors.test.ts tests/table-model.test.ts tests/table-updates.test.ts
```

Run the build first on a fresh checkout: the accessor tests import application modules that require the compiled Lingui catalogs. The focused tests then need no browser, credentials, network service or additional test dependencies. They cover missing values versus zero, totals, active series, peak accessors, record shapes, page boundaries, selection identity, local timestamps, content-change detection and the shared update registry. The standard build extracts and compiles Lingui messages, checks TypeScript and builds the frontend.

Browser acceptance should also verify the rendered table rather than only the model:

- Select Tables, reload and sign in again. The account preference should persist; Charts remains the default for accounts without a preference.
- Compare the selected metric series, units, time range, filters and average/maximum values with Charts.
- Check `table`, `caption`, column headers and row headers using a screen reader. Use table navigation to reach cards below the viewport and open metric sheets.
- Read a cell on a later page while another measurement arrives. The row, page and focus should remain unchanged until Refresh table is activated.
- Change the time range, system, series filter or units deliberately. The table should switch to the new selection, including cached and empty queries.
- Test horizontal keyboard scrolling at a narrow viewport. The scroll container is intentionally focusable; the cells are not interactive grid cells.
- Check that a failed preference save reports an error without switching the view or claiming success.

A model test or an accessibility-tree inspection is not a substitute for the last checks with a live screen reader. These tests do not claim full WCAG conformance.

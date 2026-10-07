// Package custommetrics reads numeric metrics that other programs write to
// local files, so the agent can report values Beszel does not collect itself.
//
// The metrics: section of the agent's config.yml lists sources: a directory of
// *.prom files, a file or a glob. On each collection the agent reads every
// source's files in a subset of the Prometheus text format (see Parse) and
// turns each sample into a series, which has a key (its stored identity, see
// seriesKey), a value, and metadata: its unit, help text, line name, and the
// title and description of its chart. Values go out in Stats.CustomMetrics and
// metadata in Info.CustomMetricsMeta. Counters become per-second rates by
// default. Multi-sample types, such as histograms, are skipped.
//
// The agent only reads files and never runs commands, so a producer runs with
// its own privileges and on its own schedule, and a slow one cannot delay a
// collection. Producers keep to a short contract:
//   - replace the file atomically: write a temporary file whose name does not
//     end in .prom, then rename it over the real one;
//   - rewrite it every cycle, even when nothing changed: a sample older than
//     its source's max_age is dropped, leaving a gap rather than a frozen line;
//   - write timestamps, if any, in milliseconds.
//
// Collect runs under the agent's lock, every second while someone watches the
// live view, so its work is bounded: files, bytes and samples per collection,
// series reported, the length of every text field, and warnings, at most one
// per file and problem an hour. Files are opened without blocking, so a FIFO
// cannot stall it. A source with an invalid setting is skipped, and a
// malformed config keeps the last good one. Reading from HTTP endpoints or
// plugins, if added, would have to happen outside Collect, which would read
// their last result.
package custommetrics

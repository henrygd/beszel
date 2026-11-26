# Guidelines for AI agents

These rules apply to any AI agent (or person using one) that opens issues or pull requests on Beszel. Keep it simple and keep it reviewable.

## Before you start

- **Search first.** Check open and closed issues and PRs before you start. If an open PR already covers the change, don't open another one.
- **Never open a PR for an enhancement that hasn't been approved.** Every new feature or enhancement, however small, needs an issue that the maintainer (@henrygd) has approved before you open a PR. You can build and test it locally in the meantime, but wait for approval before opening the PR. While a feature request is labelled `needs-review` it isn't approved yet. Bug fixes don't need approval first.
- **Ask before big or architectural changes.** For anything significant, such as a large new feature, a change to how the hub and agent talk to each other, a new dependency, or a restructure of existing code, discuss the approach in the issue before you write much code. A large PR that wasn't discussed first may be closed, even if the code is good.
- **Support questions go to [Discussions](https://github.com/henrygd/beszel/discussions).** Translation issues go to [Crowdin](https://crowdin.com/project/beszel). Neither belongs in the issue tracker.

## Issues

- Use the issue templates in [.github/ISSUE_TEMPLATE](.github/ISSUE_TEMPLATE). Fill in every required field and keep the template's structure and headings. Extra detail is welcome (logs, versions, steps to reproduce), but put it inside the template's fields. Don't replace the template with your own format.
- Before reporting a bug, search open **and closed** issues and [Discussions](https://github.com/henrygd/beszel/discussions) for the same problem, and check the [Common Issues guide](https://beszel.dev/guide/common-issues). Many reports turn out to be fixed by a configuration change on the user's side (environment variables, Docker mounts, permissions). If an existing answer solves it, don't open a new issue.
- One bug or one feature request per issue.
- **If you found a bug, fix it too.** Don't just open an issue and leave it there. Open the issue, then open a PR with the fix that links to it (`Fixes #123`). If you can't fix it, say why in the issue.
- Only tick the template checkboxes (docs read, existing issues searched, latest version) if you actually did those things.

## Labels

Use only the labels the issue template sets: `bug` for a bug report, and `enhancement` and `needs-review` for a feature request. Don't add other labels, don't add labels to PRs, and don't add, remove or change labels on an issue after you've opened it. The maintainer handles triage. Check an issue's labels before you work on it:

- `needs-review`: the maintainer hasn't approved the feature yet. Don't start a PR until they have.
- `out of scope` or `wontfix`: don't open a PR.
- `in progress`: someone is already working on it. Look for an open PR before you start.
- `waiting on upstream`: blocked on another project. Don't try to work around it.
- `fixed-pending-release`: already fixed on `main`. Don't report or fix it again.
- `awaiting-requester`: more information is needed. If it's your issue, reply with it, or the issue will be closed as stale.

## Comments

- Only comment on an issue or PR when you add something new, such as logs, a reproduction or a confirmed workaround. Don't post "+1" comments, restate the issue, or offer to implement something nobody asked for.
- Don't post reviews on other people's PRs unless someone asked for one.
- Don't @-mention the maintainer to ask for a review or approval, and don't ping again on an open issue or PR. They will get to it.

## Security reports

Only open a [security advisory](SECURITY.md) for a real, high-severity vulnerability that you have reproduced on the latest version. Report anything else as a normal issue. Unverified, AI-generated advisories take up maintainer time and will be closed.

## Branches and pull requests

- Create a new branch from an up-to-date `main` for each change. Never work directly on `main`.
- **One thing per PR.** A PR adds one feature or fixes one bug. Don't bundle unrelated fixes, refactors or formatting changes. Small PRs are easier and faster to review.
- **Keep the scope small.** Don't let a PR grow while you work on it. Build the smallest version that is useful on its own, and leave extras for a follow-up PR once the first one is merged. A PR that does less but is easy to review is more likely to be merged.
- **You don't have to build everything that was asked for.** An issue or feature request describes what someone would like, not a spec that must be implemented in full. If part of it doesn't fit Beszel, or would add a lot of complexity for little benefit, leave it out and explain why in the PR description.
- **Never stack PRs.** Every PR must be based on `main` and must not depend on another open PR.
- Fill in the [pull request template](.github/pull_request_template.md). Link the related issue (`Fixes #123`). For any UI fix or change, add before and after screenshots. Put your changes under the matching Changelog headings (Added, Changed, Fixed, Removed) and delete the sections you don't use. Don't leave placeholder text like "one" and "two".
- Keep the diff focused. Don't reformat, rename or "clean up" code you didn't need to touch.
- If your PR changes the hub, agent or web UI, add one line to [supplemental/CHANGELOG.md](supplemental/CHANGELOG.md) under a `## Unreleased` heading at the top (create it if it's missing). Keep the existing style, for example `- Add TLS certificate expiry alert for network monitors (#2627)`. You won't know the PR number until the PR is open, so add it in a follow-up commit. Skip it for docs, CI and tooling changes.
- Don't change version numbers. Maintainers handle those at release time.
- Don't bump dependencies unless that is the whole point of the PR. Avoid adding new dependencies unless they're clearly needed, and explain why in the PR.
- Documentation changes go to the [beszel-docs](https://github.com/henrygd/beszel-docs) repo. Link that PR in the description.
- Keep the description short and honest. Never claim tests passed or something was checked unless it actually was.
- **Say what you could and couldn't test.** Many agent fixes depend on hardware or an OS you may not have (GPUs, sensors, SMART, sysfs, Windows). State what you tested and on which hardware and OS, and what you couldn't test. Don't present a fix as verified when you couldn't reproduce the problem.
- Say in the PR description that it was made with an AI agent.
- Address review feedback on the same PR. Don't close it and open a new one.

## Code

- **Hub and agent compatibility.** Hubs and agents often run different versions. Changes to the data the agent sends, or to how the hub and agent connect, must keep working with older agents and hubs. When a feature needs a newer agent, gate it on the agent version the same way network monitors do:
  - Add a `MinVersion...` constant in [beszel.go](beszel.go), for example `MinVersionNetworkMonitors`, and check it in the hub before using the feature: `if sys.getAgentVersion().LT(beszel.MinVersionNetworkMonitors) { ... }`.
  - In the web UI, hide or disable the feature for older agents with a helper like `supportsNetworkMonitors` in [internal/site/src/lib/utils.ts](internal/site/src/lib/utils.ts).
- **CBOR keys are permanent.** Data from the agent uses integer CBOR keys, for example `cbor:"0,keyasint"` in [internal/entities/system/system.go](internal/entities/system/system.go). Never renumber, reuse or remove a key, because hubs and agents on other versions would read the wrong field. Add new fields with the next unused number and `omitempty`.
- **Keep the MCP server up to date.** The hub's MCP server in [internal/hub/mcp](internal/hub/mcp) exposes Beszel data to AI clients. If you add new data or change existing data (new metrics, fields, collections, units or record types), update the MCP tools in the same PR so they return the new or changed data, and update `mcp_test.go`.
- **Migrations.** Never edit an existing file in [internal/migrations](internal/migrations). Add a new migration instead.
- **Platforms.** The agent runs on Linux, macOS, Windows and FreeBSD. Put OS-specific code in the matching file (for example `cpu_linux.go`), and make sure your change doesn't break the other platforms. Check the agent on each OS before opening a PR:

  ```sh
  GOOS=linux go vet ./agent/...
  GOOS=darwin go vet ./agent/...
  GOOS=freebsd go vet ./agent/...
  make build-agent OS=windows   # needs the .NET SDK
  ```

  If you can't build for Windows, say so in the PR description.

## Commit messages

Start each commit message with one of these prefixes:

- `feat:` for new functionality or enhancements to existing functionality.
- `fix:` for something that was broken and is now fixed.
- `task:` for everything else (dependencies, config, refactors, docs, tests, tooling).

You can add a scope, for example `fix(agent): ...` or `feat(ui): ...`. Write the summary in the imperative mood and keep it short.

## Locales

Don't commit changes under `internal/site/src/locales/`. Building the web UI rewrites those files, and translations are managed through [Crowdin](https://crowdin.com/project/beszel). As the [developer guide](https://beszel.dev/guide/developer-guide) suggests, exclude them locally:

```sh
echo "locales/" >> .git/info/exclude
```

## Frontend

The web UI lives in [internal/site](internal/site) and uses shadcn/ui components. Install the shadcn skills before working on it:

```sh
npx skills add shadcn/ui
```

- Reuse the existing components in `internal/site/src/components/ui` before adding new ones.
- Wrap user-facing strings for translation with Lingui (`t` / `<Trans>`).
- Format and lint with Biome, using the config in `internal/site/biome.json`.

## Checks before opening a PR

Run these and make sure they pass:

```sh
# Go
make test   # go test -tags='testing no_ui' ./...
make lint   # golangci-lint run

# Frontend (from internal/site)
bun test
bunx biome check .
bun run build
```

Add or update tests for the code you changed. Don't open a PR with failing checks or code you haven't run.

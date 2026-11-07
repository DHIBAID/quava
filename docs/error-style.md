# Error style

One style everywhere, so an error reads the same in the log, the CLI and the apps.

| Rule | Example |
| --- | --- |
| Lowercase, no trailing period, one line | `cannot reach quavad: connection refused` |
| Name the component, then the step that failed, then the cause: `component: step: cause` | `session: send ping: write tcp: broken pipe` |
| Wrap the cause with `%w` (Go) or the exception cause (Kotlin) so callers can inspect it | `fmt.Errorf("config: save: %w", err)` |
| A message about bad input that is shown as-is may skip the component | `volume must be between 0.0 and 1.0` |
| Never print an error and also return it; the layer that finally shows it does that, once | |

Components in Go are the package or subsystem: `session`, `config`, `protocol`,
`discovery`, `crypto`, `pairing`, `mpris`, `audio`, `artwork`, `uinput`,
`remote input`, `clipboard`, `media`, `profiling`, `ssh`, `ipc`, `pair`.

Where the message ends up:

- **The daemon** answers a failed request with `{"error": "<message>"}`, same style, no prefix added.
- **The CLI** prints `quava: <message>` on standard error and exits with status 1. Nothing else prints errors.
- **The desktop app** shows the message in a toast, with the first letter capitalized.
- **The Android app** logs the exception (lowercase) and shows status text as a sentence, e.g. `Connection failed: <message>`.
- **Logs** read `component: what happened: cause`, for example `session: phone status: battery=100% ...`.

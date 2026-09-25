# javacard-rpc 0.4.3

The generated Java Card stream state can live in `CLEAR_ON_RESET` memory. Nothing
about the wire protocol, the instruction map or the refusals changes, and the
default output keeps `CLEAR_ON_DESELECT`.

## Added

- `jcrpc-gen --stream-memory clear_on_deselect|clear_on_reset`, and
  `codegen.GenerateJavaSkeletonWithOptions` with `JavaOptions.StreamMemory` in the
  library. `clear_on_reset` allocates the four arrays the skeleton injects into
  the stream runtime (workspace, digest scratch, scalars, handler slot) and the
  adapter's I/O scratch as `CLEAR_ON_RESET`.
- Why: a Security Domain forwards STORE DATA to an application through
  `org.globalplatform.Personalization.processData` while the Security Domain, not
  the application, is selected. `CLEAR_ON_DESELECT` memory is out of reach in
  that call, so a streamed personalization operation could not run there.
- The applet must still call the adapter's `deselect()` from its deselect
  callback, as before. With `clear_on_reset` that call is what empties the stream
  state on deselect; a card reset empties it as well.

## Not changed

- `GenerateJavaSkeleton` keeps its signature and generates the same sources as an
  explicit `clear_on_deselect`.
- An unknown `--stream-memory` value is refused with the generation exit code
  before any file is written.
- The two generated comments that named `CLEAR_ON_DESELECT` as a fixed fact now
  name the selected memory, so default output differs from 0.4.2 in those comment
  lines only.

## Evidence

- `go test ./...` in `codegen`: `TestStreamMemoryOptionSelectsTheTransientEvent`
  (each value selects its event in all five allocations and leaves none of the
  other; default equals explicit `clear_on_deselect`; an unknown value is
  refused) and `TestRunStreamMemoryFlag` (the flag reaches the generated files; an
  unknown value exits with the generation code and writes nothing).

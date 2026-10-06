# javacard-rpc 0.4.5

Urgent generator and counter-launcher corrections before facade migration.

- Regenerate streamed Java servers: the mandatory adapter change compares CLA
  logical-channel encodings while preserving class, chaining and secure-messaging
  bits. For configured `0xB6`, it accepts `0xB4..0xB7` and `0xF0..0xFE`;
  reserved `0xFF` cannot represent channel 19 for that class. This comparison
  does not authorize channels or secure messaging. Ordinary instruction fallback,
  host output and the stream wire protocol remain compatible; streams retain one
  shared owner. See [CLA limits](.spec/idl.md#53-stream-adapter-cla-comparison) and
  [Oracle Java Card RE 3.2, §4.4, Tables 4-2/4-3](https://docs.oracle.com/en/java/javacard/3.2/jc-re-spec/F74157_03.pdf).
- The bulk workspace stays transient by default. Omitted, empty and `"transient"`
  `stream_workspace` values preserve that policy. An explicit
  `[applet] stream_workspace = "persistent"` moves only the shared bulk workspace
  to NVM; `--stream-memory` still independently selects transient control-state
  lifecycle. Audit every streamed method before opting the whole applet in.
- The canonical counter launcher uses Gradle's actual bridge archive and resolved
  runtime classpath, with freshness checks that reject missing, stale, ambiguous
  or interrupted publication. It no longer expects an obsolete bridge filename.

Observed generated allocation payload for the immutable bsim-auth B6 IDL at
`2d23abdafa1e0f68c6003ab56274b2ac38378ef9` (IDL SHA-256
`1be1ed52ac9a85a62d5c5e371a9e22d38f834282681528476ca071e7bfc2cb66`):

| Constructed generated arrays | v0.4.4 | v0.4.5 transient default | v0.4.5 persistent opt-in |
| --- | ---: | ---: | ---: |
| Transient byte/short payload | 2375 B | 2375 B | 328 B |
| Persistent byte/short payload | 108 B | 108 B | 2156 B |
| Transient handler references | 1 slot | 1 slot | 1 slot |

The opt-in saves 2047 B of measured transient primitive payload: a 2048 B bulk
workspace moves to NVM and a 1 B transient reset marker is added. These are
constructed generated-array counts using the pinned relux.2 simulator, excluding
object headers, reference widths and digest-provider/JCRE/runtime allocations.
They are neither total physical-card RAM measurements nor NovaCard hardware proof.

Persistent storage is for large, infrequent **open-data** frames only. Never stage
secrets in NVM; the IDL has no secrecy annotations and cannot enforce this rule.
Uploads and handler output consume NVM writes/endurance; cleanup writes the full
workspace capacity. For 2048 B capacity, a 1792 B upload, 1792 B handler output
written once and read close account for 5632 logical NVM byte assignments;
starting another stream adds a further 2048 B wipe. These are logical writes,
not physical endurance measurements.

Reset invalidates the session through transient state but retained persistent
bytes are wiped only at the next stream dispatch before reuse. Abort, deselect
and terminal protocol cleanup wipe explicitly. Interrupted cleanup can leave
residual bytes; no atomic secure erasure or tear-resistance guarantee is made.
See [storage policy and limits](.spec/idl.md#52-bulk-workspace-storage-policy).
Consumer IDL opt-in and the physical NovaCard rerun remain consumer-owned.

The immutable v0.4.4 comparison source is peeled commit
`01926103eb7a9997f83923a2440d47d748b401a1`; annotated tag object
`bacbc37abc08f39dde9c66bb6a51de250a2c1db7` is distinct from that source commit.
Facade parity should use the delivered v0.4.5 source after signed release landing.

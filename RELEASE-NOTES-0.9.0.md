# Release 0.9.0

This release composes the signed JavaCard server 0.8.0, Kotlin client 0.3.1 and
Swift client 0.2.3. The compatibility manifest records the actual signed tag
objects, commits and runtime coordinates. Production Go modules use released
versions without local replacements.

Mixed-mode JavaCard skeletons expose `abortPublicStreams(reason)` so an applet
can discard public uploads, pending results and short receipts when its external
authority changes. Authenticated issuer streams remain intact. The applet still
owns its handler's transient state. Full `abortStreams(reason)` retains its
all-stream behavior; wire formats and method modes are unchanged.

Host qualification uses the published strict jCardSim 3.0.5.9-relux.4. Source,
native compatibility, Classic conversion and downstream consumer acceptance are
separate gates. Simulation and conversion do not qualify a physical card or COS.

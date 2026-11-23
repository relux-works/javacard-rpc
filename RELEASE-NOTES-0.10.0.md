# Release 0.10.0

This release composes JavaCard server 0.9.0, Kotlin client 0.3.1 and
Swift client 0.2.3. The compatibility manifest binds each runtime to its
signed tag object and commit.

Mixed-mode JavaCard applets can reuse the request digest that the runtime
verifies at stream close. They can also provide a transient workspace for a
public upload and its streamed result, then wipe and release that workspace
when the stream closes or aborts. These hooks are optional. Default storage,
authenticated issuer streams and wire formats retain their existing behavior.

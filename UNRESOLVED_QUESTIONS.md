# Unresolved Questions

## Swift stream client

The bounded six-command stream lifecycle is generated for Java Card and Kotlin.
Swift generation currently rejects schemas containing `stream` before writing
partial output. A future task must decide whether the Swift client needs the
same generated lifecycle and transport invalidation contract; this does not
block the BSimID Android and Java Card path.

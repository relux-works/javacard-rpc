# javacard-rpc 0.4.1

Release notes for the R3-04 remediation.

## Fixed

- Reusable generated `StatusWordException` and `StreamStatusWordException` instances now keep their current status in a one-slot `CLEAR_ON_RESET` transient `short[]`, eliminating persistent-memory writes when unauthenticated frames alternate error statuses.
- The generator's allocation/state regression covers both nested exception classes and the generated skeleton, including a 10,000-frame alternating-status endurance witness.
- Generated Java package metadata includes the Java Card simulator compile-only dependency for both ordinary and streamed skeletons.

## Compatibility

- Generated exception and stream-runtime constructor signatures remain unchanged. The generated Java Card skeleton now requires the Java Card `JCSystem` API for transient status storage, as does the generated stream endpoint.

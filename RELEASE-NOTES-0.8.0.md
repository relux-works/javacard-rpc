# Release 0.8.0

This release composes the signed JavaCard server 0.7.0, Kotlin client 0.3.1 and
Swift client 0.2.3. The compatibility manifest records the actual tag objects,
peeled commits, Go modules and runtime coordinates. Normal module acquisition
and checksum verification remain required; local replacements are refused.

The JavaCard server installs immutable dispatch metadata once and passes a
compact invocation frame. Facade property tests now exercise that released
interface, including installation rejection, metadata isolation, unknown
method behavior and recovery. Historical output parity fixtures remain
unchanged; the explicitly current SampleStore source projection is updated.

The compatibility toolchain downloads strict jCardSim 3.0.5.9-relux.4 and
verifies its digest. GCM uses the legal online initializers; the offline
eight-argument initializer rejects GCM. Classic conversion, native consumers
and compatibility probes remain separate required validation lanes.

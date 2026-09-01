# Local patch for `github.com/disgoorg/godave/golibdave`

This directory is a copy of the `golibdave` wrapper from upstream version
`v0.3.0`, licensed under Apache-2.0.

The only functional change is in `session.Decrypt`: its passthrough fallback
copies the incoming frame into the output buffer (`copy(decryptedFrame, frame)`).
Upstream's v0.3.0 release reverses those arguments, which overwrites the input
and produces invalid Opus data before a speaker-specific DAVE decryptor exists.

Remove this directory and the root `go.mod` replacement once upstream releases
the same correction.

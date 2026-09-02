# Local DAVE session wrapper

`session.go` is derived from
[`github.com/disgoorg/godave/golibdave` v0.3.0](https://github.com/disgoorg/godave),
which is licensed under Apache-2.0. It is kept in VexBot's `internal/dave`
package so the bot can use its local correction directly, without maintaining a
separate replacement Go module under `third_party`.

## Local correction

Until DAVE has created a decryptor for an individual speaker, the wrapper runs
in passthrough mode. In that state, `session.Decrypt` must copy the received
frame into the destination buffer:

```go
return copy(decryptedFrame, frame), nil
```

The upstream `v0.3.0` implementation reverses those arguments. That overwrites
the received Opus frame instead, causing corrupted-stream errors before the
speaker-specific decryptor is ready.

`session_test.go` protects this behavior with
`TestDecryptPassthroughPreservesFrame`.

## Updating or removing this copy

When an upstream `golibdave` release includes this same correction, compare its
session implementation with `session.go`. If no VexBot-specific behavior
remains, remove `session.go`, `session_test.go`, and this note; add the updated
`golibdave` dependency; and switch `voice_connection.go` back to
`golibdave.NewSession`.

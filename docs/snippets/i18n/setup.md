The Go SDK is server-side: it emits the browser i18n loader tag (public client key) for the `{{PROFILE}}` profile. Translation rendering itself happens in the browser via the client SDK's `t()`.

```go
eng := shipeasy.ConfiguredEngine()

// clientKey is the PUBLIC client key. Emit this in your document <head>.
tag := eng.I18nScriptTag(clientKey, "{{PROFILE}}", shipeasy.BootstrapTagOptions{})
```

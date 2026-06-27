The Go SDK is server-side: it emits the browser i18n loader tag (public client key) for the `{{PROFILE}}` profile. Translation rendering itself happens in the browser via the client SDK's `t()`. Assumes `Configure()` ran at startup — see Installation.

```go
// fetch the process-wide engine once per callsite (built by Configure)
eng := shipeasy.ConfiguredEngine()

// clientKey is the PUBLIC client key (NOT the server key). Emit this tag in
// your document <head>.
//   clientKey  string                  the public client key
//   "{{PROFILE}}"  string              the locale profile to load (e.g. "en:prod")
//   BootstrapTagOptions{}              optional: AnonID, I18nProfile, BaseURL
tag := eng.I18nScriptTag(clientKey, "{{PROFILE}}", shipeasy.BootstrapTagOptions{})
_ = tag
```

The Go SDK is server-side: it emits the browser i18n loader tag (public client key) for the `{{PROFILE}}` profile. Translation rendering itself happens in the browser via the client SDK's `t()`. Assumes `Configure()` ran at startup — see Installation.

```go
// I18nScriptTag is package-level — it runs off the global Configure(). Emit
// the returned tag in your document <head>.
//   clientKey      string   the PUBLIC client key (NOT the server key)
//   "{{PROFILE}}"  string   the locale profile to load (e.g. "en:prod")
//   BootstrapTagOptions{}   optional: AnonID, I18nProfile, BaseURL
tag := shipeasy.I18nScriptTag(clientKey, "{{PROFILE}}", shipeasy.BootstrapTagOptions{})
_ = tag
```

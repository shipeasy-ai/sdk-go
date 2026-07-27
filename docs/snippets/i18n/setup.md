The Go SDK is server-side: it emits the browser i18n loader tag (public client key) for the `{{PROFILE}}` profile. Translation rendering itself happens in the browser via the client SDK's `t()`. Assumes `Configure()` ran at startup — see Installation.

```go
// I18nScriptTag is package-level — it runs off the global Configure(). Emit the
// returned tag in your document <head>. EVERY argument is optional: the PUBLIC
// client key, profile and CDN origin come from Configure(Options{ClientKey: ...,
// Profile: "{{PROFILE}}", ProjectID: ...}).
//   TagOptions{}   optional: AnonID, I18nProfile, ClientKey, ProjectID,
//                  BaseURL, NoDefer — pass one to override a single tag
tag := shipeasy.I18nScriptTag()

// Devtools overlay (Shift+Alt+S or ?se=1) — opens only for a signed-in
// Shipeasy session, so gating it on staff/env is optional.
devtools := shipeasy.DevtoolsScriptTag()
_, _ = tag, devtools
```
